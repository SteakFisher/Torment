#!/bin/sh
# Runs the main API flow against a running server at BASE_URL (default http://localhost:8080), sending
# API_KEY (required) as the bearer token, and exits
# non-zero at the first failed check. Each run uses a new rider and new transfer ids, so it can be rerun
# against the same databases.
set -eu

BASE_URL=${BASE_URL:-http://localhost:8080}
API_KEY=${API_KEY:?API_KEY is not set}
run=$(date +%s)
rider="rider:$run:wallet"
bonus="rider:$run:bonus"
pay=${run}1 overspend=${run}2 refund=${run}3 eur=${run}4 move=${run}5

# post PATH BODY and get PATH print the response body, a space and the status code, on one line.
post() { curl -sS -H "Authorization: Bearer $API_KEY" -X POST -H 'Content-Type: application/json' -d "$2" -w ' %{http_code}' "$BASE_URL$1" | tr -d '\n'; }
get() { curl -sS -H "Authorization: Bearer $API_KEY" -w ' %{http_code}' "$BASE_URL$1" | tr -d '\n'; }

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

expect "no key is rejected" "$(curl -sS -w ' %{http_code}' "$BASE_URL/accounts")" '^\{"error":\{"code":"unauthorized",.* 401$'
expect "register USD" "$(post /currencies '{"code":"USD/2"}')" ' (200|201)$'
expect "register EUR" "$(post /currencies '{"code":"EUR/2"}')" ' (200|201)$'
expect "list currencies" "$(get /currencies)" '"name":"EUR","precision":2\}.*"name":"USD","precision":2\}.* 200$'
expect "register world" "$(post /accounts '{"address":"world","type":"equity","allow_negative":true}')" ' (200|201)$'
expect "register rider" "$(post /accounts "$(printf '{"address":"%s","type":"liability"}' "$rider")")" ' 201$'
expect "account in an unregistered currency" "$(post /accounts "$(printf '{"address":"%s","type":"liability","currencies":["NOPE"]}' "$bonus")")" '"code":"currency_not_found".* 404$'
expect "transfer in an unregistered currency" "$(post /transfers "$(posting "$pay" world "$rider" NOPE 1)")" '"code":"currency_not_found".* 404$'
expect "transfer from an unregistered account" "$(post /transfers "$(posting "$pay" "nobody:$run" "$rider" USD 1)")" '"code":"account_not_found".* 404$'
expect "unknown account" "$(get "/accounts/nobody:$run")" '"code":"account_not_found".* 404$'

expect "pay 100 USD" "$(post /transfers "$(posting "$pay" world "$rider" USD 10000)")" '"status":"created".* 200$'
expect "rider has 100 USD" "$(get "/accounts/$rider/balances")" '"currency":"USD","precision":2,"amount":"10000".* 200$'

expect "resend pay" "$(post /transfers "$(posting "$pay" world "$rider" USD 10000)")" '"status":"exists".* 200$'
expect "rider still has 100 USD" "$(get "/accounts/$rider/balances")" '"amount":"10000".* 200$'

expect "overspend" "$(post /transfers "$(posting "$overspend" "$rider" world USD 20000)")" '"error":"insufficient_funds".* 200$'

expect "refund 30 USD in the pay group" "$(post /transfers "$(posting "$refund" "$rider" world USD 3000 "$pay")")" '"status":"created".* 200$'
expect "group has refund then pay" "$(get "/groups/$pay/transfers")" "\"id\":\"$refund\".*\"id\":\"$pay\".* 200\$"

expect "pay 5 EUR" "$(post /transfers "$(posting "$eur" world "$rider" EUR 500)")" '"status":"created".* 200$'
expect "rider has EUR and USD" "$(get "/accounts/$rider/balances")" '"currency":"EUR","precision":2,"amount":"500".*"currency":"USD","precision":2,"amount":"7000".* 200$'
expect "rider EUR balance only" "$(get "/accounts/$rider/balances?currency=EUR")" '"balances":\[\{"currency":"EUR","precision":2,"amount":"500"\}\]\} 200$'

# at_pay is the time TB committed the pay transfer.
at_pay=$(get "/groups/$pay/transfers" | sed -E "s/.*\"id\":\"$pay\"[^}]*\"timestamp\":\"([^\"]+)\".*/\1/")
expect "rider had 100 USD at pay" "$(get "/accounts/$rider/balances?at=$at_pay")" '"currency":"USD","precision":2,"amount":"10000".* 200$'

expect "get rider" "$(get "/accounts/$rider")" "^\\{\"address\":\"$rider\",\"type\":\"liability\",.*\"wallets\":\\[\\{\"currency\":\"EUR\",\"precision\":2\\},\\{\"currency\":\"USD\",\"precision\":2\\}\\]\\} 200\$"
expect "list rider accounts" "$(get "/accounts/rider:$run:*")" "\"address\":\"$rider\".*\"currency\":\"EUR\".*\"currency\":\"USD\".* 200\$"
expect "rider totals" "$(get "/accounts/rider:$run:*/balances")" '"totals":\[\{"type":"liability","currency":"EUR","precision":2,"amount":"500"\},\{"type":"liability","currency":"USD","precision":2,"amount":"7000"\}\].* 200$'

page=$(get "/accounts/$rider/transfers?currency=USD&limit=1")
expect "newest USD transfer is the refund" "$page" "\"id\":\"$refund\".*\"next\":.* 200\$"
before=$(printf '%s' "$page" | sed -E 's/.*"next":"([^"]+)".*/\1/')
expect "next page is the pay" "$(get "/accounts/$rider/transfers?currency=USD&limit=1&before=$before")" "\"id\":\"$pay\".* 200\$"

expect "register bonus" "$(post /accounts "$(printf '{"address":"%s","type":"liability"}' "$bonus")")" ' 201$'
expect "move 10 USD to bonus" "$(post /transfers "$(posting "$move" "$rider" "$bonus" USD 1000)")" '"status":"created".* 200$'
pots=$(get "/accounts/rider:$run:*/transfers")
expect "pattern transfers merge the rider's pots" "$pots" "\"id\":\"$move\".*\"id\":\"$eur\".*\"id\":\"$refund\".*\"id\":\"$pay\".* 200\$"
expect "move between pots appears once" "$(printf '%s' "$pots" | grep -o "\"id\":\"$move\"" | wc -l | tr -d ' ')" '^1$'
