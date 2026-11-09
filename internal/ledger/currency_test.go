package ledger

import (
	"context"
	"errors"
	"testing"
)

func TestParseCurrencyCode(t *testing.T) {
	tests := []struct {
		code      string
		name      string
		precision int16
		wantErr   bool
	}{
		{code: "USD/2", name: "USD", precision: 2},
		{code: "JPY", name: "JPY", precision: 0},
		{code: "BTC/8", name: "BTC", precision: 8},
		{code: "USD_C/18", name: "USD_C", precision: 18},
		{code: "A1/0", name: "A1", precision: 0},
		{code: "", wantErr: true},
		{code: "usd/2", wantErr: true},
		{code: "1USD", wantErr: true},
		{code: "_USD", wantErr: true},
		{code: "USD/", wantErr: true},
		{code: "USD/19", wantErr: true},
		{code: "USD/02", wantErr: true},
		{code: "USD/00", wantErr: true},
		{code: "USD/-1", wantErr: true},
		{code: "USD/2/3", wantErr: true},
		{code: "US D", wantErr: true},
		{code: "USD/99999999999999999999", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			name, precision, err := ParseCurrencyCode(tt.code)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("err = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if name != tt.name || precision != tt.precision {
				t.Fatalf("got (%q, %d), want (%q, %d)", name, precision, tt.name, tt.precision)
			}
		})
	}
}

func TestRegisterCurrency(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		stored      *Currency // the currency already stored under the name, if any
		wantErr     error
		wantCall    bool
		wantCreated bool
	}{
		{name: "invalid code", code: "usd", wantErr: ErrInvalid},
		{name: "created", code: "USD/2", wantCall: true, wantCreated: true},
		{name: "created without precision", code: "JPY", wantCall: true, wantCreated: true},
		{name: "replay", code: "USD/2", stored: &Currency{Name: "USD", Precision: 2, Ledger: 7}, wantCall: true},
		{name: "different precision", code: "USD/4", stored: &Currency{Name: "USD", Precision: 2, Ledger: 7}, wantErr: ErrConflict, wantCall: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			store := &fakeStore{insertCurrency: func(_ context.Context, name string, precision int16) (Currency, bool, error) {
				called = true
				if tt.stored != nil {
					return *tt.stored, false, nil
				}
				return Currency{Name: name, Precision: precision, Ledger: 7}, true, nil
			}}
			got, created, err := New(store, nil, nil).RegisterCurrency(context.Background(), tt.code)
			if called != tt.wantCall {
				t.Fatalf("InsertCurrency called = %v, want %v", called, tt.wantCall)
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			name, precision, _ := ParseCurrencyCode(tt.code)
			if want := (Currency{Name: name, Precision: precision, Ledger: 7}); got != want || created != tt.wantCreated {
				t.Fatalf("got %+v, created %v; want %+v, %v", got, created, want, tt.wantCreated)
			}
		})
	}
}
