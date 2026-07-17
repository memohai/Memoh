package store

import "errors"

type persistenceRetrySafeError struct {
	err error
}

func (e *persistenceRetrySafeError) Error() string {
	return e.err.Error()
}

func (e *persistenceRetrySafeError) Unwrap() error {
	return e.err
}

func (*persistenceRetrySafeError) PersistenceRetrySafe() bool {
	return true
}

// MarkPersistenceRetrySafe identifies an error that occurred before a
// transaction could reach commit and can therefore be retried without
// duplicating durable writes.
func MarkPersistenceRetrySafe(err error) error {
	if err == nil || IsPersistenceRetrySafe(err) {
		return err
	}
	return &persistenceRetrySafeError{err: err}
}

// IsPersistenceRetrySafe reports whether persistence can be retried without
// an ambiguous-commit reconciliation step.
func IsPersistenceRetrySafe(err error) bool {
	var marked interface {
		PersistenceRetrySafe() bool
	}
	return errors.As(err, &marked) && marked.PersistenceRetrySafe()
}
