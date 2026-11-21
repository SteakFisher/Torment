#!/bin/sh
# Runs every request of the brokerage tutorial (docs/src/content/docs/brokerage) against a running server
# at BASE_URL (default http://localhost:8080), sending API_KEY (required) as the bearer token, and exits
# non-zero at the first failed check. Each run uses new users and ids, so it can be rerun against the same
# data. Balances are checked only on the run's own users, since the brokerage's accounts carry every run.
# When RABBITMQ_API is set (the RabbitMQ management API, with the credentials of a user that may declare
# consumer.* queues), it also checks the notification events of chapter 10, so the exporter must be
# running; those checks need jq.
set -eu

BASE_URL=${BASE_URL:-http://localhost:8080}
API_KEY=${API_KEY:?API_KEY is not set}
RABBITMQ_API=${RABBITMQ_API:-}
run=$(date +%s)
user="user:$run"
amira="user:${run}a" omar="user:${run}o"

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

# id N prints this run's id N as a UUID: the run in the first group, N in the last; ids grow with N.
id() { printf '%08x-0000-0000-0000-%012d' "$run" "$1"; }

# account ADDRESS TYPE [EXTRA] prints an account registration, with EXTRA appended as JSON fields.
account() { printf '{"address":"%s","type":"%s"%s}' "$1" "$2" "${3:+,$3}"; }

# p ID DEBIT CREDIT CURRENCY AMOUNT KIND [GROUP_ID] prints one posting.
p() { printf '{"id":"%s","debit":"%s","credit":"%s","currency":"%s","amount":"%s","kind":"%s","group_id":"%s"}' "$1" "$2" "$3" "$4" "$5" "$6" "${7:-}"; }

# kind NAME registers the kind NAME.
kind() { expect "register kind $1" "$(post /kinds "{\"name\":\"$1\"}")" "^\\{\"name\":\"$1\"\\} (200|201)\$"; }

# transfer CHAIN... posts one request with a chain per argument. Each CHAIN is comma-separated postings.
transfer() {
	body="[$1]"
	shift
	for c in "$@"; do body="$body,[$c]"; done
	post /transfers "{\"chains\":[$body]}"
}

# amount ADDRESS CURRENCY prints the account's balance in currency.
amount() { get "/accounts/$1/balances?currency=$2" | sed -E 's/.*"amount":"([^"]+)".*/\1/'; }

# mq ARGS... calls the RabbitMQ management API and prints the response body, a space and the status code.
mq() { curl -sS -H 'Content-Type: application/json' -w ' %{http_code}' "$@" | tr -d '\n'; }
notifications="consumer.notifications.$run"

# Chapter 10's queue is bound before the first transfer, since it gets only events published after.
if [ -n "$RABBITMQ_API" ]; then
	echo "# 10. Notifications: the queue"
	expect "declare the notifications queue" "$(mq -X PUT -d '{"durable":false,"arguments":{"x-expires":300000}}' "$RABBITMQ_API/api/queues/torment/$notifications")" ' 201$'
	for key in '*.deposit' '*.dividend' '*.withdrawal'; do
		expect "bind it to $key" "$(mq -X POST -d "{\"routing_key\":\"$key\"}" "$RABBITMQ_API/api/bindings/torment/e/torment.events/q/$notifications")" ' 201$'
	done
fi

echo "# 1. Chart of accounts"
for code in USD/2 AAPL/6 TSLA/6; do
	expect "register $code" "$(post /currencies "{\"code\":\"$code\"}")" ' (200|201)$'
done
expect "register bank:client-money" "$(post /accounts "$(account bank:client-money asset)")" ' (200|201)$'
expect "register custodian:positions" "$(post /accounts "$(account custodian:positions asset)")" ' (200|201)$'
expect "register revenue:commissions" "$(post /accounts "$(account revenue:commissions revenue)")" ' (200|201)$'
expect "registering again returns 200" "$(post /accounts "$(account bank:client-money asset)")" ' 200$'
expect "register the user's cash with a USD wallet" "$(post /accounts "$(account "$user:cash" liability '"currencies":["USD"]')")" '"wallets":\[\{"currency":"USD","precision":2\}\]\} 201$'
expect "register the user's holdings" "$(post /accounts "$(account "$user:holdings" liability)")" '"wallets":\[\]\} 201$'

echo "# 2. Deposits"
kind deposit
deposit=$(p "$(id 10)" bank:client-money "$user:cash" USD 100000 deposit)
expect "deposit 1000 USD" "$(transfer "$deposit")" '"status":"created".* 200$'
expect "the webhook arrives again" "$(transfer "$deposit")" '"status":"exists".* 200$'
expect "the user has 1000 USD" "$(amount "$user:cash" USD)" '^100000$'

echo "# 3. Buying shares"
kind trade
kind commission
buy=$(id 20)
expect "buy 1.5 AAPL at 150 with a 1 USD fee" "$(transfer "$(p "$(id 21)" "$user:cash" bank:client-money USD 22500 trade "$buy"),$(p "$(id 22)" custodian:positions "$user:holdings" AAPL 1500000 trade "$buy"),$(p "$(id 23)" "$user:cash" revenue:commissions USD 100 commission "$buy")")" '^\{"chains":\[\[(\{[^}]*"status":"created"\},?){3}\]\]\} 200$'
expect "the user has 774 USD" "$(amount "$user:cash" USD)" '^77400$'
expect "the user has 1.5 AAPL" "$(amount "$user:holdings" AAPL)" '^1500000$'
big=$(id 24)
expect "buying 10 TSLA is refused" "$(transfer "$(p "$(id 25)" "$user:cash" bank:client-money USD 200000 trade "$big"),$(p "$(id 26)" custodian:positions "$user:holdings" TSLA 10000000 trade "$big"),$(p "$(id 27)" "$user:cash" revenue:commissions USD 100 commission "$big")")" '"error":"insufficient_funds".*"error":"chain_failed".*"error":"chain_failed".* 200$'
expect "the user still has 774 USD" "$(amount "$user:cash" USD)" '^77400$'
sell=$(id 28)
expect "sell 0.5 AAPL at 160" "$(transfer "$(p "$(id 29)" "$user:holdings" custodian:positions AAPL 500000 trade "$sell"),$(p "$(id 30)" bank:client-money "$user:cash" USD 8000 trade "$sell"),$(p "$(id 31)" "$user:cash" revenue:commissions USD 100 commission "$sell")")" '^\{"chains":\[\[(\{[^}]*"status":"created"\},?){3}\]\]\} 200$'
expect "the user has 853 USD" "$(amount "$user:cash" USD)" '^85300$'
expect "the user has 1 AAPL" "$(amount "$user:holdings" AAPL)" '^1000000$'

echo "# 4. Partial fills"
order=$(id 40)
expect "fill 1: 1 TSLA at 200" "$(transfer "$(p "$(id 41)" "$user:cash" bank:client-money USD 20000 trade "$order"),$(p "$(id 42)" custodian:positions "$user:holdings" TSLA 1000000 trade "$order")")" '"status":"created".*"status":"created".* 200$'
expect "fill 2: 1 TSLA at 200.50" "$(transfer "$(p "$(id 43)" "$user:cash" bank:client-money USD 20050 trade "$order"),$(p "$(id 44)" custodian:positions "$user:holdings" TSLA 1000000 trade "$order")")" '"status":"created".*"status":"created".* 200$'
expect "fill 3: 1 TSLA at 199.75, and the fee" "$(transfer "$(p "$(id 45)" "$user:cash" bank:client-money USD 19975 trade "$order"),$(p "$(id 46)" custodian:positions "$user:holdings" TSLA 1000000 trade "$order"),$(p "$(id 47)" "$user:cash" revenue:commissions USD 100 commission "$order")")" '"status":"created".*"status":"created".*"status":"created".* 200$'
fills=$(get "/groups/$order/transfers")
expect "the order has 7 transfers" "$(printf '%s' "$fills" | grep -o '"id":' | wc -l | tr -d ' ')" '^7$'
expect "the order's newest transfer is the fee" "$fills" "^\\{\"group_id\":\"$order\",\"transfers\":\\[\\{\"id\":\"$(id 47)\"[^}]*\"kind\":\"commission\""
expect "the user has 3 TSLA" "$(amount "$user:holdings" TSLA)" '^3000000$'
expect "the user has 251.75 USD" "$(amount "$user:cash" USD)" '^25175$'

echo "# 5. Currency exchange"
kind fx
expect "register AED/2" "$(post /currencies '{"code":"AED/2"}')" ' (200|201)$'
expect "register fx:position" "$(post /accounts "$(account fx:position equity '"allow_negative":true')")" ' (200|201)$'
expect "deposit 1000 AED" "$(transfer "$(p "$(id 50)" bank:client-money "$user:cash" AED 100000 deposit)")" '"status":"created".* 200$'
fx=$(id 51)
expect "exchange 367.25 AED for 100 USD" "$(transfer "$(p "$(id 52)" "$user:cash" fx:position AED 36725 fx "$fx"),$(p "$(id 53)" fx:position "$user:cash" USD 10000 fx "$fx")")" '"status":"created".*"status":"created".* 200$'
expect "the user has 351.75 USD and 632.75 AED" "$(get "/accounts/$user:cash/balances")" '"totals":\[\{"type":"liability","currency":"AED","precision":2,"amount":"63275"\},\{"type":"liability","currency":"USD","precision":2,"amount":"35175"\}\],"accounts":\[\{"address":"[^"]+","type":"liability","balances":\[\{"currency":"AED","precision":2,"amount":"63275"\},\{"currency":"USD","precision":2,"amount":"35175"\}\]\}\]\} 200$'
settle=$(id 54)
expect "treasury converts at the bank" "$(transfer "$(p "$(id 55)" fx:position bank:client-money AED 36725 fx "$settle"),$(p "$(id 56)" bank:client-money fx:position USD 10000 fx "$settle")")" '"status":"created".*"status":"created".* 200$'

echo "# 6. Dividends"
kind dividend
for u in "$amira" "$omar"; do
	expect "register $u:cash" "$(post /accounts "$(account "$u:cash" liability)")" ' 201$'
done
dividend=$(id 60)
expect "pay the TSLA dividend to three users" "$(transfer "$(p "$(id 61)" bank:client-money "$user:cash" USD 150 dividend "$dividend")" "$(p "$(id 62)" bank:client-money "$amira:cash" USD 1200 dividend "$dividend")" "$(p "$(id 63)" bank:client-money "$omar:cash" USD 25 dividend "$dividend")")" '^\{"chains":\[(\[\{[^}]*"status":"created"\}\],?){3}\]\} 200$'
expect "the dividend group lists three payments" "$(get "/groups/$dividend/transfers" | grep -o '"id":' | wc -l | tr -d ' ')" '^3$'
expect "the user has 353.25 USD" "$(amount "$user:cash" USD)" '^35325$'

echo "# 7. Withdrawals"
kind withdrawal
expect "register transit:withdrawals" "$(post /accounts "$(account transit:withdrawals liability)")" ' (200|201)$'
w1=$(id 70)
expect "the user asks to withdraw 100 USD" "$(transfer "$(p "$(id 71)" "$user:cash" transit:withdrawals USD 10000 withdrawal "$w1")")" '"status":"created".* 200$'
expect "the user has 253.25 USD" "$(amount "$user:cash" USD)" '^25325$'
expect "the bank pays it out" "$(transfer "$(p "$(id 72)" transit:withdrawals bank:client-money USD 10000 withdrawal "$w1")")" '"status":"created".* 200$'
w2=$(id 73)
expect "the user asks to withdraw 50 USD" "$(transfer "$(p "$(id 74)" "$user:cash" transit:withdrawals USD 5000 withdrawal "$w2")")" '"status":"created".* 200$'
expect "the bank rejects it, so it goes back" "$(transfer "$(p "$(id 75)" transit:withdrawals "$user:cash" USD 5000 withdrawal "$w2")")" '"status":"created".* 200$'
expect "the user still has 253.25 USD" "$(amount "$user:cash" USD)" '^25325$'

echo "# 8. Statements"
page=$(get "/accounts/$user:*/transfers?limit=5")
expect "the newest 5 transfers start with the returned withdrawal" "$page" "^\\{\"address\":\"$user:\\*\",\"transfers\":\\[\\{\"id\":\"$(id 75)\".*\"next\":\"[^\"]+\"\\} 200\$"
cursor=$(printf '%s' "$page" | sed -E 's/.*"next":"([^"]+)".*/\1/')
expect "the next page carries on" "$(get "/accounts/$user:*/transfers?limit=5&cursor=$cursor")" "^\\{\"address\":\"$user:\\*\",\"transfers\":\\[\\{\"id\":\"$(id 52)\""
expect "the user's dividends are the one payment" "$(get "/accounts/$user:*/transfers?kind=dividend")" "^\\{\"address\":\"$user:\\*\",\"transfers\":\\[\\{\"id\":\"$(id 61)\"[^}]*\"kind\":\"dividend\"[^}]*\\}\\]\\} 200\$"
at=$(get "/groups/$(id 10)/transfers" | sed -E 's/.*"timestamp":"([^"]+)".*/\1/')
expect "the balance right after the deposit was 1000 USD" "$(get "/accounts/$user:cash/balances?currency=USD&at=$at")" '"amount":"100000".* 200$'
expect "the user's totals" "$(get "/accounts/$user:*/balances")" '"totals":\[\{"type":"liability","currency":"AAPL","precision":6,"amount":"1000000"\},\{"type":"liability","currency":"AED","precision":2,"amount":"63275"\},\{"type":"liability","currency":"TSLA","precision":6,"amount":"3000000"\},\{"type":"liability","currency":"USD","precision":2,"amount":"25325"\}\]'

echo "# 9. Mistakes"
kind correction
expect "fill 1 was really at 190: reverse it and post it again" "$(transfer "$(p "$(id 91)" bank:client-money "$user:cash" USD 20000 correction "$order"),$(p "$(id 92)" "$user:cash" bank:client-money USD 19000 correction "$order")")" '"status":"created".*"status":"created".* 200$'
expect "the user has 263.25 USD" "$(amount "$user:cash" USD)" '^26325$'
expect "the order now has 9 transfers" "$(get "/groups/$order/transfers" | grep -o '"id":' | wc -l | tr -d ' ')" '^9$'
expect "reusing an id with other fields is a conflict" "$(transfer "$(p "$(id 10)" bank:client-money "$user:cash" USD 999 deposit)")" '"error":"conflict".* 200$'

if [ -n "$RABBITMQ_API" ]; then
	echo "# 10. Notifications"
	# got holds the queue's events as "<routing key> <id> <debit> <credit> <credit balance>" lines, read
	# until the returned withdrawal, the run's last notification, arrives or 10 seconds pass.
	got=""
	for _ in $(seq 1 20); do
		got="$got
$(curl -sS -X POST -H 'Content-Type: application/json' -d '{"count":1000,"ackmode":"ack_requeue_false","encoding":"auto"}' \
	"$RABBITMQ_API/api/queues/torment/$notifications/get" |
	jq -r '.[] | .payload | fromjson | "\(.currency).\(.kind) \(.id) \(.debit) \(.credit) \(.credit_balance)"')"
		printf '%s' "$got" | grep -q "^USD.withdrawal $(id 75) " && break
		sleep 0.5
	done
	expect "the deposit arrives (is the exporter running?)" "$got" "^USD\.deposit $(id 10) bank:client-money $user:cash 100000\$"
	expect "the AED deposit arrives" "$got" "^AED\.deposit $(id 50) bank:client-money $user:cash "
	expect "the dividend carries the balance after it" "$got" "^USD\.dividend $(id 61) bank:client-money $user:cash 35325\$"
	expect "the withdrawal request" "$got" "^USD\.withdrawal $(id 71) $user:cash transit:withdrawals "
	expect "the payout names no user" "$got" "^USD\.withdrawal $(id 72) transit:withdrawals bank:client-money "
	expect "the payout's group names the user" "$(get "/groups/$w1/transfers")" "\"id\":\"$(id 71)\",\"group_id\":\"$w1\",\"debit\":\"$user:cash\""
	expect "the rejected withdrawal goes back" "$got" "^USD\.withdrawal $(id 75) transit:withdrawals $user:cash 25325\$"
	expect "trades and commissions are not in the queue" "$(printf '%s' "$got" | grep -Ec '^[A-Z]+\.(trade|commission|fx|correction) ' || true)" '^0$'
	expect "delete the notifications queue" "$(mq -X DELETE "$RABBITMQ_API/api/queues/torment/$notifications")" ' 204$'
fi
