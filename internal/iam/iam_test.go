package iam

import (
	"context"
	"strings"
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/policy"
	"github.com/google/uuid"
)

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern, s string
		want       bool
	}{
		{"*", "anything", true},
		{"ap:*", "ap:InvokeModel", true},
		{"ap:Invoke*", "ap:InvokeModel", true},
		{"ap:Invoke*", "ap:InvokeChat", true},
		{"ap:Invoke*", "ap:ListUsers", false},
		{"ap:InvokeModel", "ap:InvokeModel", true},
		{"ap:InvokeModel", "ap:InvokeModelX", false},
		{"arn:ap:model/*", "arn:ap:model/gpt-4o", true},
		{"arn:ap:model/*", "arn:ap:provider/openai", false},
		{"arn:ap:*/gpt-4o", "arn:ap:model/gpt-4o", true},
		{"ap:Invoke?odel", "ap:InvokeModel", true},
		{"ap:Invoke?odel", "ap:InvokeXXodel", false},
		{"", "ap:InvokeModel", false},
		{"ap:*", "", false},
		{"*", "", false},
	}
	for _, tc := range cases {
		if got := MatchAction(tc.pattern, tc.s); got != tc.want {
			t.Errorf("MatchAction(%q, %q) = %v, want %v", tc.pattern, tc.s, got, tc.want)
		}
	}
}

func TestValidActionPattern(t *testing.T) {
	valid := []string{"*", "ap:InvokeModel", "ap:Invoke*", "ap:*", "ap:Create_Thing"}
	for _, s := range valid {
		if !ValidActionPattern(s) {
			t.Errorf("expected %q to be valid", s)
		}
	}
	invalid := []string{"", "InvokeModel", "aws:Invoke", "ap:", "ap:Invoke Model", "ap:invoke-model"}
	for _, s := range invalid {
		if ValidActionPattern(s) {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestValidResourcePattern(t *testing.T) {
	valid := []string{"*", "arn:ap:*", "arn:ap:model/*", "arn:ap:model/gpt-4o", "arn:ap:provider/*", "arn:ap:model/gpt-?"}
	for _, s := range valid {
		if !ValidResourcePattern(s) {
			t.Errorf("expected %q to be valid", s)
		}
	}
	invalid := []string{"", "arn:aws:s3:::bucket", "arn:ap:", "arn:ap:model", "arn:ap:model/", "model/gpt-4o"}
	for _, s := range invalid {
		if ValidResourcePattern(s) {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestParseDocument_StringAndArrayForms(t *testing.T) {
	// Action/Resource as single strings, Statement as a single object.
	singleObject := []byte(`{
		"Version": "2026-01-01",
		"Statement": {"Effect": "Allow", "Action": "ap:InvokeChat", "Resource": "*"}
	}`)
	doc, err := ParseDocument(singleObject)
	if err != nil {
		t.Fatalf("parse single-object document: %v", err)
	}
	if len(doc.Statement) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(doc.Statement))
	}
	if len(doc.Statement[0].Action) != 1 || doc.Statement[0].Action[0] != "ap:InvokeChat" {
		t.Fatalf("unexpected action: %#v", doc.Statement[0].Action)
	}

	// Arrays.
	arrayForm := []byte(`{
		"Statement": [{"Effect": "Allow", "Action": ["ap:InvokeChat"], "Resource": ["arn:ap:*"]}]
	}`)
	if _, err := ParseDocument(arrayForm); err != nil {
		t.Fatalf("parse array document: %v", err)
	}
}

func TestParseDocument_RejectsUnknownFields(t *testing.T) {
	if _, err := ParseDocument([]byte(`{"Statment": []}`)); err == nil {
		t.Fatal("expected error for misspelled field")
	}
}

func TestValidate(t *testing.T) {
	valid := func(doc string) error {
		d, err := ParseDocument([]byte(doc))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return Validate(d, ValidateOptions{})
	}

	t.Run("accepts a well-formed document", func(t *testing.T) {
		err := valid(`{"Statement":[
			{"Sid":"a","Effect":"Allow","Action":["ap:InvokeChat","ap:InvokeModel"],"Resource":["arn:ap:*"]},
			{"Sid":"b","Effect":"Deny","Action":"ap:InvokeModel","Resource":"arn:ap:model/gpt-4o"}
		]}`)
		if err != nil {
			t.Fatalf("expected valid, got %v", err)
		}
	})

	t.Run("rejects no statements", func(t *testing.T) {
		if err := valid(`{"Statement":[]}`); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("rejects bad effect", func(t *testing.T) {
		if err := valid(`{"Statement":[{"Effect":"Permit","Action":"ap:InvokeChat","Resource":"*"}]}`); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("rejects Action and NotAction together", func(t *testing.T) {
		if err := valid(`{"Statement":[{"Effect":"Allow","Action":"ap:InvokeChat","NotAction":"ap:InvokeModel","Resource":"*"}]}`); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("rejects missing Resource", func(t *testing.T) {
		if err := valid(`{"Statement":[{"Effect":"Allow","Action":"ap:InvokeChat"}]}`); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("rejects unknown action", func(t *testing.T) {
		if err := valid(`{"Statement":[{"Effect":"Allow","Action":"ap:LaunchRockets","Resource":"*"}]}`); err == nil {
			t.Fatal("expected error for unknown action")
		}
	})

	t.Run("allows unknown action when opted in", func(t *testing.T) {
		d, _ := ParseDocument([]byte(`{"Statement":[{"Effect":"Allow","Action":"ap:LaunchRockets","Resource":"*"}]}`))
		if err := Validate(d, ValidateOptions{AllowUnknownActions: true}); err != nil {
			t.Fatalf("expected valid, got %v", err)
		}
	})

	t.Run("rejects malformed resource", func(t *testing.T) {
		if err := valid(`{"Statement":[{"Effect":"Allow","Action":"ap:InvokeChat","Resource":"not-an-arn"}]}`); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("rejects duplicate Sid", func(t *testing.T) {
		if err := valid(`{"Statement":[
			{"Sid":"dup","Effect":"Allow","Action":"ap:InvokeChat","Resource":"*"},
			{"Sid":"dup","Effect":"Deny","Action":"ap:InvokeChat","Resource":"*"}
		]}`); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("rejects conditions in v1", func(t *testing.T) {
		if err := valid(`{"Statement":[{"Effect":"Deny","Action":"ap:InvokeChat","Resource":"*","Condition":{"IpAddress":{"ap:SourceIp":"1.2.3.4"}}}]}`); err == nil {
			t.Fatal("expected error for unsupported condition")
		}
	})

	t.Run("rejects oversized document", func(t *testing.T) {
		big := strings.Repeat("x", MaxDocumentBytes)
		doc := `{"Statement":[{"Effect":"Allow","Action":"ap:InvokeChat","Resource":"arn:ap:model/` + big + `"}]}`
		d, err := ParseDocument([]byte(doc))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if err := Validate(d, ValidateOptions{}); err == nil {
			t.Fatal("expected size error")
		}
	})
}

func doc(t *testing.T, s string) *PolicyDocument {
	t.Helper()
	d, err := ParseDocument([]byte(s))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(d, ValidateOptions{}); err != nil {
		t.Fatalf("validate: %v", err)
	}
	return d
}

func TestEvaluateDocuments(t *testing.T) {
	req := Request{Action: ActionInvokeModel, Resource: "arn:ap:model/gpt-4o"}

	t.Run("implicit deny with no policies", func(t *testing.T) {
		if got := EvaluateDocuments(nil, req); got != DecisionDeny {
			t.Fatalf("expected implicit deny, got %s", got)
		}
	})

	t.Run("allow on matching action and resource", func(t *testing.T) {
		d := doc(t, `{"Statement":[{"Effect":"Allow","Action":"ap:InvokeModel","Resource":"arn:ap:model/*"}]}`)
		if got := EvaluateDocuments([]*PolicyDocument{d}, req); got != DecisionAllow {
			t.Fatalf("expected allow, got %s", got)
		}
	})

	t.Run("implicit deny when action does not match", func(t *testing.T) {
		d := doc(t, `{"Statement":[{"Effect":"Allow","Action":"ap:InvokeChat","Resource":"*"}]}`)
		if got := EvaluateDocuments([]*PolicyDocument{d}, req); got != DecisionDeny {
			t.Fatalf("expected deny, got %s", got)
		}
	})

	t.Run("implicit deny when resource does not match", func(t *testing.T) {
		d := doc(t, `{"Statement":[{"Effect":"Allow","Action":"ap:InvokeModel","Resource":"arn:ap:model/other"}]}`)
		if got := EvaluateDocuments([]*PolicyDocument{d}, req); got != DecisionDeny {
			t.Fatalf("expected deny, got %s", got)
		}
	})

	t.Run("explicit deny overrides allow in another policy", func(t *testing.T) {
		allow := doc(t, `{"Statement":[{"Effect":"Allow","Action":"ap:*","Resource":"arn:ap:*"}]}`)
		deny := doc(t, `{"Statement":[{"Effect":"Deny","Action":"ap:InvokeModel","Resource":"arn:ap:model/gpt-4o"}]}`)
		if got := EvaluateDocuments([]*PolicyDocument{allow, deny}, req); got != DecisionDeny {
			t.Fatalf("expected deny (explicit deny wins), got %s", got)
		}
	})

	t.Run("explicit deny overrides allow in the same policy", func(t *testing.T) {
		d := doc(t, `{"Statement":[
			{"Effect":"Allow","Action":"ap:*","Resource":"arn:ap:*"},
			{"Effect":"Deny","Action":"ap:InvokeModel","Resource":"arn:ap:model/gpt-4o"}
		]}`)
		if got := EvaluateDocuments([]*PolicyDocument{d}, req); got != DecisionDeny {
			t.Fatalf("expected deny, got %s", got)
		}
	})

	t.Run("deny with non-matching action does not override allow", func(t *testing.T) {
		d := doc(t, `{"Statement":[
			{"Effect":"Allow","Action":"ap:InvokeModel","Resource":"arn:ap:*"},
			{"Effect":"Deny","Action":"ap:InvokeChat","Resource":"*"}
		]}`)
		if got := EvaluateDocuments([]*PolicyDocument{d}, req); got != DecisionAllow {
			t.Fatalf("expected allow, got %s", got)
		}
	})

	t.Run("NotAction grants everything except the listed action", func(t *testing.T) {
		d := doc(t, `{"Statement":[{"Effect":"Allow","NotAction":"ap:InvokeChat","Resource":"*"}]}`)
		if got := EvaluateDocuments([]*PolicyDocument{d}, req); got != DecisionAllow {
			t.Fatalf("expected allow, got %s", got)
		}
		chatReq := Request{Action: ActionInvokeChat, Resource: "*"}
		if got := EvaluateDocuments([]*PolicyDocument{d}, chatReq); got != DecisionDeny {
			t.Fatalf("expected deny for NotAction-excluded action, got %s", got)
		}
	})

	t.Run("NotResource matches everything except the listed resource", func(t *testing.T) {
		d := doc(t, `{"Statement":[{"Effect":"Deny","Action":"ap:InvokeModel","NotResource":"arn:ap:model/safe"}]}`)
		if got := EvaluateDocuments([]*PolicyDocument{d}, req); got != DecisionDeny {
			t.Fatalf("expected deny (resource not excluded), got %s", got)
		}
		safeReq := Request{Action: ActionInvokeModel, Resource: "arn:ap:model/safe"}
		if got := EvaluateDocuments([]*PolicyDocument{d}, safeReq); got != DecisionDeny {
			t.Fatalf("expected implicit deny (deny does not apply), got %s", got)
		}
	})
}

func TestEvaluateWithBoundary(t *testing.T) {
	req := Request{Action: ActionInvokeModel, Resource: "arn:ap:model/gpt-4o"}

	allowAll := doc(t, `{"Statement":[{"Effect":"Allow","Action":"ap:*","Resource":"arn:ap:*"}]}`)

	t.Run("boundary caps an otherwise broad allow", func(t *testing.T) {
		boundary := doc(t, `{"Statement":[{"Effect":"Allow","Action":"ap:InvokeChat","Resource":"*"}]}`)
		got := EvaluateWithBoundary([]*PolicyDocument{allowAll}, boundary, req)
		if got != DecisionDeny {
			t.Fatalf("expected deny (boundary does not allow InvokeModel), got %s", got)
		}
	})

	t.Run("boundary that allows the action permits the identity allow", func(t *testing.T) {
		boundary := doc(t, `{"Statement":[{"Effect":"Allow","Action":"ap:*","Resource":"arn:ap:*"}]}`)
		got := EvaluateWithBoundary([]*PolicyDocument{allowAll}, boundary, req)
		if got != DecisionAllow {
			t.Fatalf("expected allow, got %s", got)
		}
	})

	t.Run("boundary cannot grant what identity policies deny (implicit)", func(t *testing.T) {
		// identity allows nothing; boundary allows everything → still implicit deny
		boundary := doc(t, `{"Statement":[{"Effect":"Allow","Action":"ap:*","Resource":"arn:ap:*"}]}`)
		got := EvaluateWithBoundary(nil, boundary, req)
		if got != DecisionDeny {
			t.Fatalf("expected implicit deny, got %s", got)
		}
	})

	t.Run("explicit deny in boundary wins", func(t *testing.T) {
		boundary := doc(t, `{"Statement":[{"Effect":"Deny","Action":"ap:InvokeModel","Resource":"arn:ap:model/gpt-4o"}]}`)
		got := EvaluateWithBoundary([]*PolicyDocument{allowAll}, boundary, req)
		if got != DecisionDeny {
			t.Fatalf("expected deny, got %s", got)
		}
	})

	t.Run("no boundary behaves like EvaluateDocuments", func(t *testing.T) {
		got := EvaluateWithBoundary([]*PolicyDocument{allowAll}, nil, req)
		if got != DecisionAllow {
			t.Fatalf("expected allow, got %s", got)
		}
	})
}

type fakePolicyLoader struct {
	policies []*PolicyDocument
	boundary *PolicyDocument
	err      error
}

func (f fakePolicyLoader) LoadPrincipalPolicies(context.Context, uuid.UUID) ([]*PolicyDocument, error) {
	return f.policies, f.err
}

func (f fakePolicyLoader) LoadPermissionsBoundary(context.Context, uuid.UUID) (*PolicyDocument, error) {
	return f.boundary, f.err
}

func TestEngine(t *testing.T) {
	allow := doc(t, `{"Statement":[{"Effect":"Allow","Action":"ap:InvokeModel","Resource":"arn:ap:model/gpt-4o"}]}`)

	t.Run("allows a matching request", func(t *testing.T) {
		e := New(fakePolicyLoader{policies: []*PolicyDocument{allow}})
		decision, err := e.Evaluate(context.Background(), policy.Request{
			UserID:   uuid.New(),
			Action:   ActionInvokeModel,
			Resource: "arn:ap:model/gpt-4o",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if decision != policy.DecisionAllow {
			t.Fatalf("expected allow, got %s", decision)
		}
	})

	t.Run("denies an unmatched request", func(t *testing.T) {
		e := New(fakePolicyLoader{policies: []*PolicyDocument{allow}})
		decision, err := e.Evaluate(context.Background(), policy.Request{
			UserID:   uuid.New(),
			Action:   ActionInvokeModel,
			Resource: "arn:ap:model/gpt-4o-mini",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if decision != policy.DecisionDeny {
			t.Fatalf("expected deny, got %s", decision)
		}
	})

	t.Run("nil user denies without loading", func(t *testing.T) {
		e := New(fakePolicyLoader{err: errSentinel})
		decision, err := e.Evaluate(context.Background(), policy.Request{Action: ActionInvokeChat})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if decision != policy.DecisionDeny {
			t.Fatalf("expected deny, got %s", decision)
		}
	})

	t.Run("loader error fails closed and propagates", func(t *testing.T) {
		e := New(fakePolicyLoader{err: errSentinel})
		decision, err := e.Evaluate(context.Background(), policy.Request{
			UserID: uuid.New(),
			Action: ActionInvokeChat,
		})
		if decision != policy.DecisionDeny {
			t.Fatalf("expected deny, got %s", decision)
		}
		if err == nil {
			t.Fatal("expected error to propagate")
		}
	})

	t.Run("boundary caps engine result", func(t *testing.T) {
		broad := doc(t, `{"Statement":[{"Effect":"Allow","Action":"ap:*","Resource":"arn:ap:*"}]}`)
		boundary := doc(t, `{"Statement":[{"Effect":"Allow","Action":"ap:InvokeChat","Resource":"*"}]}`)
		e := New(fakePolicyLoader{policies: []*PolicyDocument{broad}, boundary: boundary})
		decision, _ := e.Evaluate(context.Background(), policy.Request{
			UserID:   uuid.New(),
			Action:   ActionInvokeModel,
			Resource: "arn:ap:model/gpt-4o",
		})
		if decision != policy.DecisionDeny {
			t.Fatalf("expected deny from boundary, got %s", decision)
		}
	})
}

var errSentinel = &sentinelError{}

type sentinelError struct{}

func (*sentinelError) Error() string { return "db down" }
