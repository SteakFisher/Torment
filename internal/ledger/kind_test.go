package ledger

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestValidateKindName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{name: "deposit"},
		{name: "a"},
		{name: "top_up_2"},
		{name: "a" + strings.Repeat("b", 62)},
		{name: "a" + strings.Repeat("b", 63), wantErr: true},
		{name: "", wantErr: true},
		{name: "Deposit", wantErr: true},
		{name: "1st", wantErr: true},
		{name: "_x", wantErr: true},
		{name: "top-up", wantErr: true},
		{name: "top up", wantErr: true},
		{name: "dépôt", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateKindName(tt.name)
			if tt.wantErr != errors.Is(err, ErrInvalid) || !tt.wantErr && err != nil {
				t.Fatalf("err = %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegisterKind(t *testing.T) {
	tests := []struct {
		name        string
		kind        string
		stored      bool // whether the kind is already stored
		wantErr     error
		wantCall    bool
		wantCreated bool
	}{
		{name: "invalid name", kind: "Deposit", wantErr: ErrInvalid},
		{name: "created", kind: "deposit", wantCall: true, wantCreated: true},
		{name: "replay", kind: "deposit", stored: true, wantCall: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			store := &fakeStore{insertKind: func(_ context.Context, name string) (Kind, bool, error) {
				called = true
				return Kind{Name: name, Code: 3}, !tt.stored, nil
			}}
			got, created, err := New(store, nil, nil).RegisterKind(context.Background(), tt.kind)
			if called != tt.wantCall {
				t.Fatalf("InsertKind called = %v, want %v", called, tt.wantCall)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if want := (Kind{Name: tt.kind, Code: 3}); got != want || created != tt.wantCreated {
				t.Fatalf("got %+v, created %v; want %+v, %v", got, created, want, tt.wantCreated)
			}
		})
	}
}

func TestKinds(t *testing.T) {
	store := &fakeStore{listKinds: func(context.Context) ([]Kind, error) { return testKinds, nil }}
	got, err := New(store, nil, nil).Kinds(context.Background())
	if err != nil || !reflect.DeepEqual(got, testKinds) {
		t.Fatalf("Kinds = %v, %v; want %v", got, err, testKinds)
	}
}
