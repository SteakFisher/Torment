package ledger

import (
	"context"
	"fmt"
	"regexp"
)

// Kind maps a transfer kind name to one TigerBeetle transfer Code.
type Kind struct {
	Name string
	Code uint16
}

// kindNameRE matches a kind name: lowercase letters, digits and underscores, starting with a letter, at
// most 63 bytes.
var kindNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// validateKindName returns ErrInvalid when name is not a valid kind name.
func validateKindName(name string) error {
	if !kindNameRE.MatchString(name) {
		return fmt.Errorf("%w: kind %q must be 1 to 63 lowercase letters, digits or underscores, starting with a letter", ErrInvalid, name)
	}
	return nil
}

// RegisterKind stores name as a new kind on a newly assigned TigerBeetle code. It is idempotent: when
// the name is already registered, it returns the stored kind with created false. It returns ErrInvalid
// for a malformed name.
func (s *Service) RegisterKind(ctx context.Context, name string) (k Kind, created bool, err error) {
	if err := validateKindName(name); err != nil {
		return Kind{}, false, err
	}
	return s.store.InsertKind(ctx, name)
}

// Kinds returns every registered kind, ordered by name.
func (s *Service) Kinds(ctx context.Context) ([]Kind, error) {
	return s.store.ListKinds(ctx)
}

// lookupKinds returns each named kind, keyed by name. It returns ErrKindNotFound naming the first kind
// that is not registered.
func (s *Service) lookupKinds(ctx context.Context, names []string) (map[string]Kind, error) {
	kinds, err := s.store.Kinds(ctx, names)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if _, ok := kinds[name]; !ok {
			return nil, fmt.Errorf("%w: kind %q is not registered", ErrKindNotFound, name)
		}
	}
	return kinds, nil
}

// filterCode returns the TigerBeetle code of the kind name, or 0 when name is empty. It returns
// ErrKindNotFound when the kind is not registered.
func (s *Service) filterCode(ctx context.Context, name string) (uint16, error) {
	if name == "" {
		return 0, nil
	}
	kinds, err := s.lookupKinds(ctx, []string{name})
	if err != nil {
		return 0, err
	}
	return kinds[name].Code, nil
}
