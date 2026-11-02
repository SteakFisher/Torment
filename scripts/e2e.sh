#!/bin/sh
# Runs the main API flow against a running server at BASE_URL (default http://localhost:8080) and exits
# non-zero at the first failed check. Each run uses a new rider and new transfer ids, so it can be rerun
# against the same databases.
set -eu

BASE_URL=${BASE_URL:-http://localhost:8080}
run=$(date +%s)
rider="rider:$run:wallet"
pay=${run}1 overspend=${run}2 refund=${run}3 eur=${run}4

# post PATH BODY and get PATH print the response body, a space and the status code, on one line.
post() { curl -sS -X POST -H 'Content-Type: application/json' -d "$2" -w ' %{http_code}' "$BASE_URL$1" | tr -d '\n'; }
get() { curl -sS -w ' %{http_code}' "$BASE_URL$1" | tr -d '\n'; }

# expect NAME OUTPUT PATTERN fails unless OUTPUT matches the extended regex PATTERN.
expect() {
	if printf '%s' "$2" | grep -Eq -- "$3"; then
		echo "ok   $1"
	else
		echo "FAIL $1: $2" >&2
		exit 1
	fi
}

# posting ID SOURCE DESTINATION CURRENCY AMOUNT [GROUP_ID] prints a transfer request with one posting.
posting() {
	printf '{"postings":[{"id":"%s","source":"%s","destination":"%s","currency":"%s","amount":"%s","group_id":"%s"}]}' "$1" "$2" "$3" "$4" "$5" "${6:-}"
}

expect "register USD" "$(post /currencies '{"code":"USD/2"}')" ' (201|409)$'
expect "register EUR" "$(post /currencies '{"code":"EUR/2"}')" ' (201|409)$'
expect "register world" "$(post /accounts '{"address":"world","type":"equity","allow_negative":true}')" ' (200|201)$'
expect "register rider" "$(post /accounts "$(printf '{"address":"%s","type":"liability"}' "$rider")")" ' 201$'

expect "pay 100 USD" "$(post /transfers "$(posting "$pay" world "$rider" USD 10000)")" '"status":"created".* 200$'
expect "rider has 100 USD" "$(get "/accounts/$rider/balances")" '"currency":"USD","precision":2,"amount":"10000".* 200$'

expect "resend pay" "$(post /transfers "$(posting "$pay" world "$rider" USD 10000)")" '"status":"exists".* 200$'
expect "rider still has 100 USD" "$(get "/accounts/$rider/balances")" '"amount":"10000".* 200$'

expect "overspend" "$(post /transfers "$(posting "$overspend" "$rider" world USD 20000)")" '"error":"insufficient_funds".* 200$'

expect "refund 30 USD in the pay group" "$(post /transfers "$(posting "$refund" "$rider" world USD 3000 "$pay")")" '"status":"created".* 200$'
expect "group has pay then refund" "$(get "/groups/$pay/transfers")" "\"id\":\"$pay\".*\"id\":\"$refund\".* 200\$"

expect "pay 5 EUR" "$(post /transfers "$(posting "$eur" world "$rider" EUR 500)")" '"status":"created".* 200$'
expect "rider has EUR and USD" "$(get "/accounts/$rider/balances")" '"currency":"EUR","precision":2,"amount":"500".*"currency":"USD","precision":2,"amount":"7000".* 200$'
