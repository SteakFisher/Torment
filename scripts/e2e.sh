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
# id N prints transfer id N of this run as a UUID: the run in the first group, N in the last.
id() { printf '%08x-0000-0000-0000-%012x' "$run" "$1"; }
pay=$(id 1) overspend=$(id 2) refund=$(id 3) eur=$(id 4) move=$(id 5)

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

# posting ID DEBIT CREDIT CURRENCY AMOUNT KIND [GROUP_ID] prints a transfer request with one chain of one posting.
posting() {
	printf '{"chains":[[{"id":"%s","debit":"%s","credit":"%s","currency":"%s","amount":"%s","kind":"%s","group_id":"%s"}]]}' "$1" "$2" "$3" "$4" "$5" "$6" "${7:-}"
}

expect "no key is rejected" "$(curl -sS -w ' %{http_code}' "$BASE_URL/accounts" | tr -d '\n')" '^\{"error":\{"code":"unauthorized",.* 401$'
expect "register USD" "$(post /currencies '{"code":"USD/2"}')" ' (200|201)$'
expect "register EUR" "$(post /currencies '{"code":"EUR/2"}')" ' (200|201)$'
expect "list currencies" "$(get /currencies)" '"name":"EUR","precision":2\}.*"name":"USD","precision":2\}.* 200$'
for kind in top_up refund move; do
	expect "register kind $kind" "$(post /kinds "{\"name\":\"$kind\"}")" "^\\{\"name\":\"$kind\"\\} (200|201)\$"
done
expect "register a kind again" "$(post /kinds '{"name":"top_up"}')" '^\{"name":"top_up"\} 200$'
expect "malformed kind" "$(post /kinds '{"name":"Top-Up"}')" '"code":"invalid_request".* 400$'
expect "list kinds" "$(get /kinds)" '"name":"move"\}.*"name":"refund"\}.*"name":"top_up"\}.* 200$'
expect "register world" "$(post /accounts '{"address":"world","type":"equity","allow_negative":true}')" ' (200|201)$'
expect "register rider" "$(post /accounts "$(printf '{"address":"%s","type":"liability"}' "$rider")")" ' 201$'
expect "account in an unregistered currency" "$(post /accounts "$(printf '{"address":"%s","type":"liability","currencies":["NOPE"]}' "$bonus")")" '"code":"currency_not_found".* 404$'
expect "transfer in an unregistered currency" "$(post /transfers "$(posting "$pay" world "$rider" NOPE 1 top_up)")" '"code":"currency_not_found".* 404$'
expect "transfer from an unregistered account" "$(post /transfers "$(posting "$pay" "nobody:$run" "$rider" USD 1 top_up)")" '"code":"account_not_found".* 404$'
expect "transfer without a kind" "$(post /transfers "$(posting "$pay" world "$rider" USD 1 '')")" '"code":"invalid_request","message":"chain 0 posting 0: kind is required".* 400$'
expect "transfer of an unregistered kind" "$(post /transfers "$(posting "$pay" world "$rider" USD 1 nope)")" '"code":"kind_not_found".* 404$'
expect "unknown route" "$(get /nope)" '^\{"error":\{"code":"route_not_found",.* 404$'
expect "wrong method" "$(curl -sS -H "Authorization: Bearer $API_KEY" -X DELETE -w ' %{http_code}' "$BASE_URL/currencies" | tr -d '\n')" '^\{"error":\{"code":"method_not_allowed",.* 405$'
expect "decimal id is rejected" "$(post /transfers "$(posting 1 world "$rider" USD 1 top_up)")" '"code":"invalid_request","message":"chain 0 posting 0: id \\"1\\" must be a UUID.* 400$'
expect "unknown account" "$(get "/accounts/nobody:$run")" '"code":"account_not_found".* 404$'

expect "pay 100 USD" "$(post /transfers "$(posting "$pay" world "$rider" USD 10000 top_up)")" '"status":"created".* 200$'
expect "rider has 100 USD" "$(get "/accounts/$rider/balances")" "^\\{\"address\":\"$rider\",\"totals\":\\[\\{\"type\":\"liability\",\"currency\":\"USD\",\"precision\":2,\"amount\":\"10000\"\\}\\],\"accounts\":\\[\\{\"address\":\"$rider\",\"type\":\"liability\",\"balances\":\\[\\{\"currency\":\"USD\",\"precision\":2,\"amount\":\"10000\"\\}\\]\\}\\]\\} 200\$"

expect "resend pay" "$(post /transfers "$(posting "$pay" world "$rider" USD 10000 top_up)")" '"status":"exists".* 200$'
expect "rider still has 100 USD" "$(get "/accounts/$rider/balances")" '"amount":"10000".* 200$'

expect "overspend" "$(post /transfers "$(posting "$overspend" "$rider" world USD 20000 refund)")" '"error":"insufficient_funds".* 200$'

expect "refund 30 USD in the pay group" "$(post /transfers "$(posting "$refund" "$rider" world USD 3000 refund "$pay")")" '"status":"created".* 200$'
expect "group has refund then pay" "$(get "/groups/$pay/transfers")" "\"id\":\"$refund\"[^}]*\"kind\":\"refund\".*\"id\":\"$pay\"[^}]*\"kind\":\"top_up\".* 200\$"
expect "group filtered to top_up is the pay" "$(get "/groups/$pay/transfers?kind=top_up")" "^\\{\"group_id\":\"$pay\",\"transfers\":\\[\\{\"id\":\"$pay\"[^}]*\\}\\]\\} 200\$"
expect "group filter by an unregistered kind" "$(get "/groups/$pay/transfers?kind=nope")" '"code":"kind_not_found".* 404$'

expect "pay 5 EUR" "$(post /transfers "$(posting "$eur" world "$rider" EUR 500 top_up)")" '"status":"created".* 200$'
expect "rider has EUR and USD" "$(get "/accounts/$rider/balances")" '"currency":"EUR","precision":2,"amount":"500".*"currency":"USD","precision":2,"amount":"7000".* 200$'
expect "rider EUR balance only" "$(get "/accounts/$rider/balances?currency=EUR")" '"totals":\[\{"type":"liability","currency":"EUR","precision":2,"amount":"500"\}\],"accounts":\[\{"address":"[^"]+","type":"liability","balances":\[\{"currency":"EUR","precision":2,"amount":"500"\}\]\}\]\} 200$'

# at_pay is the time TB committed the pay transfer.
at_pay=$(get "/groups/$pay/transfers" | sed -E "s/.*\"id\":\"$pay\"[^}]*\"timestamp\":\"([^\"]+)\".*/\1/")
expect "rider had 100 USD at pay" "$(get "/accounts/$rider/balances?at=$at_pay")" '"currency":"USD","precision":2,"amount":"10000".* 200$'

expect "get rider" "$(get "/accounts/$rider")" "^\\{\"accounts\":\\[\\{\"address\":\"$rider\",\"type\":\"liability\",.*\"wallets\":\\[\\{\"currency\":\"EUR\",\"precision\":2\\},\\{\"currency\":\"USD\",\"precision\":2\\}\\]\\}\\]\\} 200\$"
expect "list rider accounts" "$(get "/accounts/rider:$run:*")" "\"address\":\"$rider\".*\"currency\":\"EUR\".*\"currency\":\"USD\".* 200\$"
expect "rider totals" "$(get "/accounts/rider:$run:*/balances")" '"totals":\[\{"type":"liability","currency":"EUR","precision":2,"amount":"500"\},\{"type":"liability","currency":"USD","precision":2,"amount":"7000"\}\].* 200$'

page=$(get "/accounts/$rider/transfers?currency=USD&limit=1")
expect "newest USD transfer is the refund" "$page" "\"id\":\"$refund\".*\"next\":.* 200\$"
cursor=$(printf '%s' "$page" | sed -E 's/.*"next":"([^"]+)".*/\1/')
expect "next page is the pay" "$(get "/accounts/$rider/transfers?currency=USD&limit=1&cursor=$cursor")" "\"id\":\"$pay\".* 200\$"

expect "register bonus" "$(post /accounts "$(printf '{"address":"%s","type":"liability"}' "$bonus")")" ' 201$'
expect "bonus without wallets is listed" "$(get "/accounts/$bonus/balances")" "^\\{\"address\":\"$bonus\",\"totals\":\\[\\],\"accounts\":\\[\\{\"address\":\"$bonus\",\"type\":\"liability\",\"balances\":\\[\\]\\}\\]\\} 200\$"
expect "first page of rider accounts is the bonus" "$(get "/accounts/rider:$run:*?limit=1")" "^\\{\"accounts\":\\[\\{\"address\":\"$bonus\".*\"next\":\"$bonus\"\\} 200\$"
expect "next page of rider accounts is the wallet" "$(get "/accounts/rider:$run:*?limit=1&cursor=$bonus")" "^\\{\"accounts\":\\[\\{\"address\":\"$rider\"[^]]*\\]\\}\\]\\} 200\$"
expect "move 10 USD to bonus" "$(post /transfers "$(posting "$move" "$rider" "$bonus" USD 1000 move)")" '"status":"created".* 200$'
pots=$(get "/accounts/rider:$run:*/transfers")
expect "pattern transfers merge the rider's pots" "$pots" "\"id\":\"$move\".*\"id\":\"$eur\".*\"id\":\"$refund\".*\"id\":\"$pay\".* 200\$"
refunds=$(get "/accounts/rider:$run:*/transfers?kind=refund")
expect "the rider's refunds are the refund only" "$refunds" "^\\{\"address\":\"rider:$run:\\*\",\"transfers\":\\[\\{\"id\":\"$refund\"[^}]*\"kind\":\"refund\"[^}]*\\}\\]\\} 200\$"
expect "account filter by an unregistered kind" "$(get "/accounts/$rider/transfers?kind=nope")" '"code":"kind_not_found".* 404$'
expect "move between pots appears once" "$(printf '%s' "$pots" | grep -o "\"id\":\"$move\"" | wc -l | tr -d ' ')" '^1$'
