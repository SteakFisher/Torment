package ledger_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/db"
	"torment/internal/httpapi"
	"torment/internal/ledger"
)

// queryCounter is a pgx.QueryTracer that counts the queries started on its connections.
type queryCounter struct{ n atomic.Int64 }

var _ pgx.QueryTracer = (*queryCounter)(nil)

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.n.Add(1)
	return ctx
}

func (c *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// bench holds a Service on its own traced pool, the kind "bench", the currency USD, an allow_negative
// source account and a destination account, both under prefix, which is unique to the bench.
type bench struct {
	s       *ledger.Service
	queries *queryCounter
	prefix  string
	src     string
	dst     string
}

// newBench returns a bench with fresh accounts, skipping unless TEST_DATABASE_URL and TEST_TB_ADDRESS
// are both set. The test database is migrated by TestMain and is not truncated.
func newBench(b *testing.B) *bench {
	b.Helper()
	if testPool == nil || testTB == nil {
		b.Skip("TEST_DATABASE_URL or TEST_TB_ADDRESS is not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		b.Fatalf("error parsing database url: %v", err)
	}
	queries := &queryCounter{}
	cfg.ConnConfig.Tracer = queries
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		b.Fatalf("error creating pool: %v", err)
	}
	b.Cleanup(pool.Close)

	s := ledger.New(db.NewStore(pool), testTB, testTB)
	if _, _, err := s.RegisterKind(ctx, "bench"); err != nil {
		b.Fatalf("error registering kind: %v", err)
	}
	if _, _, err := s.RegisterCurrency(ctx, "USD/2"); err != nil {
		b.Fatalf("error registering currency: %v", err)
	}
	prefix := "bench:" + ledger.FormatID(tb.ID())
	bn := &bench{s: s, queries: queries, prefix: prefix, src: prefix + ":src", dst: prefix + ":dst"}
	bn.account(b, bn.src, true)
	bn.account(b, bn.dst, false)
	return bn
}

// account registers a USD account at address.
func (bn *bench) account(b *testing.B, address string, allowNegative bool) {
	b.Helper()
	p := ledger.RegisterAccountParams{Address: address, Type: "liability", AllowNegative: allowNegative, Currencies: []string{"USD"}}
	if _, _, err := bn.s.RegisterAccount(context.Background(), p); err != nil {
		b.Fatalf("error registering account %s: %v", address, err)
	}
}

// posting returns a posting of 1 USD minor unit from debit to credit with a new id.
func posting(debit, credit string) ledger.Posting {
	return ledger.Posting{ID: ledger.FormatID(tb.ID()), Debit: debit, Credit: credit, Currency: "USD", Amount: "1", Kind: "bench"}
}

// transfer posts chains and returns the first request or posting error.
func (bn *bench) transfer(chains [][]ledger.Posting) error {
	results, err := bn.s.Transfer(context.Background(), chains)
	if err != nil {
		return fmt.Errorf("error transferring: %w", err)
	}
	for _, chain := range results {
		for _, t := range chain {
			if t.Err != nil {
				return fmt.Errorf("error posting %s: %w", ledger.FormatID(t.ID), t.Err)
			}
		}
	}
	return nil
}

// start zeroes the query count and the timer; call it once setup is done.
func (bn *bench) start(b *testing.B) {
	bn.queries.n.Store(0)
	b.ResetTimer()
}

// report adds the queries per op counted since start.
func (bn *bench) report(b *testing.B) {
	b.ReportMetric(float64(bn.queries.n.Load())/float64(b.N), "pg-queries/op")
}

func BenchmarkTransfer(b *testing.B) {
	bn := newBench(b)
	if err := bn.transfer([][]ledger.Posting{{posting(bn.src, bn.dst)}}); err != nil {
		b.Fatal(err)
	}
	bn.start(b)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := bn.transfer([][]ledger.Posting{{posting(bn.src, bn.dst)}}); err != nil {
				b.Error(err)
				return
			}
		}
	})
	bn.report(b)
}

func BenchmarkTransferBatch(b *testing.B) {
	bn := newBench(b)
	if err := bn.transfer([][]ledger.Posting{{posting(bn.src, bn.dst)}}); err != nil {
		b.Fatal(err)
	}
	bn.start(b)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			chains := make([][]ledger.Posting, 100)
			for i := range chains {
				chains[i] = []ledger.Posting{posting(bn.src, bn.dst)}
			}
			if err := bn.transfer(chains); err != nil {
				b.Error(err)
				return
			}
		}
	})
	bn.report(b)
}

func BenchmarkAccountTransfers(b *testing.B) {
	bn := newBench(b)
	chains := make([][]ledger.Posting, 50)
	for i := range chains {
		chains[i] = []ledger.Posting{posting(bn.src, bn.dst)}
	}
	if err := bn.transfer(chains); err != nil {
		b.Fatal(err)
	}
	bn.start(b)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			got, _, err := bn.s.AccountTransfers(context.Background(), ledger.AccountTransfersParams{Address: bn.dst})
			if err != nil || len(got) != len(chains) {
				b.Errorf("account transfers = %d, err %v; want %d", len(got), err, len(chains))
				return
			}
		}
	})
	bn.report(b)
}

func BenchmarkPatternBalances(b *testing.B) {
	bn := newBench(b)
	const accounts = 50
	chains := make([][]ledger.Posting, accounts)
	for i := range chains {
		address := fmt.Sprintf("%s:p:%d", bn.prefix, i)
		bn.account(b, address, false)
		chains[i] = []ledger.Posting{posting(bn.src, address)}
	}
	if err := bn.transfer(chains); err != nil {
		b.Fatal(err)
	}
	pattern := bn.prefix + ":p:*"
	bn.start(b)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, got, err := bn.s.Balances(context.Background(), pattern, "", time.Time{})
			if err != nil || len(got) != accounts {
				b.Errorf("pattern balances = %d accounts, err %v; want %d", len(got), err, accounts)
				return
			}
		}
	})
	bn.report(b)
}

func BenchmarkHTTPTransfer(b *testing.B) {
	bn := newBench(b)
	if err := bn.transfer([][]ledger.Posting{{posting(bn.src, bn.dst)}}); err != nil {
		b.Fatal(err)
	}
	logger := slog.Default()
	slog.SetDefault(slog.New(slog.DiscardHandler))
	b.Cleanup(func() { slog.SetDefault(logger) })
	h := httpapi.New(bn.s, []string{"bench-key"})
	bn.start(b)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			body := fmt.Sprintf(`{"chains":[[{"id":%q,"debit":%q,"credit":%q,"currency":"USD","amount":"1","kind":"bench"}]]}`, ledger.FormatID(tb.ID()), bn.src, bn.dst)
			req := httptest.NewRequest(http.MethodPost, "/transfers", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer bench-key")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"created"`) {
				b.Errorf("POST /transfers = %d %s", rec.Code, rec.Body)
				return
			}
		}
	})
	bn.report(b)
}
