package consent

import (
	"context"
	"fmt"
)

type InputError struct {
	Message string
}

func (e *InputError) Error() string { return e.Message }

type OperationKind string

const (
	BuildEvent       OperationKind = "build_event"
	ReleaseOperation OperationKind = "release_operation"
	DiagnosticAccess OperationKind = "diagnostic_access"
)

type Operation struct {
	EventID  string        `json:"event_id"`
	UserID   string        `json:"user_id"`
	Kind     OperationKind `json:"kind"`
	Category string        `json:"category"`
}

type Decision struct {
	EventID  string        `json:"event_id"`
	Kind     OperationKind `json:"kind"`
	Allowed  bool          `json:"allowed"`
	Reason   string        `json:"reason"`
	Category string        `json:"category"`
}

type Checker interface {
	Check(context.Context, string, string) (bool, error)
}

type ConsentWriter interface {
	Grant(context.Context, string, string, string, string) error
	Revoke(context.Context, string, string, string) error
}

func AuthorizeOperation(ctx context.Context, checker Checker, operation Operation) (Decision, error) {
	if err := validateOperation(operation); err != nil {
		return Decision{}, err
	}
	granted, err := checker.Check(ctx, operation.UserID, operation.Category)
	if err != nil {
		return Decision{}, err
	}
	reason := "consent_granted"
	if !granted {
		reason = "consent_required"
	}
	return Decision{
		EventID: operation.EventID, Kind: operation.Kind, Allowed: granted,
		Reason: reason, Category: operation.Category,
	}, nil
}

type Change struct {
	EventID  string `json:"event_id"`
	UserID   string `json:"user_id"`
	Category string `json:"category"`
	Action   string `json:"action"`
	Source   string `json:"source,omitempty"`
}

type ChangeResult struct {
	EventID  string `json:"event_id"`
	Category string `json:"category"`
	State    string `json:"state"`
}

func ApplyChange(ctx context.Context, writer ConsentWriter, change Change) (ChangeResult, error) {
	if change.EventID == "" || change.UserID == "" || change.Category == "" {
		return ChangeResult{}, &InputError{Message: "event_id, user_id, and category are required"}
	}
	key := fmt.Sprintf("%s:%s:%s", change.EventID, change.Category, change.Action)
	switch change.Action {
	case "grant":
		if change.Source == "" {
			return ChangeResult{}, &InputError{Message: "source is required for grant"}
		}
		if err := writer.Grant(ctx, change.UserID, change.Category, change.Source, key); err != nil {
			return ChangeResult{}, err
		}
		return ChangeResult{EventID: change.EventID, Category: change.Category, State: "granted"}, nil
	case "revoke":
		if err := writer.Revoke(ctx, change.UserID, change.Category, key); err != nil {
			return ChangeResult{}, err
		}
		return ChangeResult{EventID: change.EventID, Category: change.Category, State: "revoked"}, nil
	default:
		return ChangeResult{}, &InputError{Message: "action must be grant or revoke"}
	}
}

func validateOperation(operation Operation) error {
	if operation.EventID == "" || operation.UserID == "" || operation.Category == "" {
		return &InputError{Message: "event_id, user_id, and category are required"}
	}
	switch operation.Kind {
	case BuildEvent, ReleaseOperation, DiagnosticAccess:
		return nil
	default:
		return &InputError{Message: "kind must be build_event, release_operation, or diagnostic_access"}
	}
}
