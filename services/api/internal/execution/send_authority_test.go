package execution

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/arbion/platform/services/api/internal/risk"
)

func TestExecutionRiskEvidencePreservesManualJSONAndRecordsAISource(t *testing.T) {
	decision := risk.RiskEvaluation{Mode: "MANUAL_PROPOSAL", ApprovalRequired: true}
	original, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	manual, err := json.Marshal(executionRiskEvidence{RiskEvaluation: decision})
	if err != nil || !bytes.Equal(original, manual) {
		t.Fatal("execution-local evidence changed manual risk JSON", err)
	}
	autonomous, err := json.Marshal(executionRiskEvidence{RiskEvaluation: decision, Source: risk.SourceAI})
	if err != nil || !bytes.Contains(autonomous, []byte(`"Source":"AI"`)) {
		t.Fatal("autonomous evidence omitted AI source", err)
	}
}

func TestSendAuthorityCannotSelectOtherConsentBranch(t *testing.T) {
	r := requestFixture()
	owner, mandate := NewOwnerAuthority(nil), NewMandateAuthority(nil)
	if !matchesSendAuthority(owner, r) || matchesSendAuthority(mandate, r) || matchesSendAuthority(nil, r) {
		t.Fatal("owner request admitted through another authority")
	}
	r.MandateApprovalID = r.OwnerID
	if matchesSendAuthority(owner, r) || !matchesSendAuthority(mandate, r) {
		t.Fatal("autonomous request admitted through owner authority")
	}
	r.MandateApprovalID = "invalid"
	if matchesSendAuthority(owner, r) || matchesSendAuthority(mandate, r) {
		t.Fatal("invalid autonomous request admitted")
	}
}

func TestFinalSendBindsSavedAuthorityBranch(t *testing.T) {
	r := requestFixture()
	ownerApproval, mandateApproval, mandateID, version := r.ClientOrderID, r.ConnectionID, r.CapitalBucketID, 2
	manual := executionRiskEvidence{RiskEvaluation: risk.RiskEvaluation{UserID: r.OwnerID, AccountID: r.AccountID, Decision: risk.Allow, Mode: "MANUAL_PROPOSAL", ApprovalRequired: true}}
	autonomous := executionRiskEvidence{RiskEvaluation: risk.RiskEvaluation{UserID: r.OwnerID, AccountID: r.AccountID, Decision: risk.Allow, Mode: "LIVE", MandateID: &mandateID, MandateVersion: &version}, Source: risk.SourceAI}
	ownerChecked := checkedOwnerAuthority{order: Order{Request: r}, approvalID: ownerApproval, decision: manual.RiskEvaluation}
	autoChecked := checkedOwnerAuthority{order: Order{Request: r}, mandateApprovalID: mandateApproval, decision: autonomous.RiskEvaluation}
	autoChecked.order.Request.MandateApprovalID = mandateApproval
	if !sameSendAuthority(&ownerApproval, nil, manual, ownerChecked) || !sameSendAuthority(nil, &mandateApproval, autonomous, autoChecked) {
		t.Fatal("exact saved authority branch rejected")
	}
	for _, checked := range []checkedOwnerAuthority{ownerChecked, autoChecked} {
		for _, saved := range []executionRiskEvidence{manual, autonomous} {
			if sameSendAuthority(nil, nil, saved, checked) || sameSendAuthority(&ownerApproval, &mandateApproval, saved, checked) {
				t.Fatal("missing or ambiguous consent branch accepted")
			}
		}
	}
	if sameSendAuthority(nil, &mandateApproval, autonomous, ownerChecked) || sameSendAuthority(&ownerApproval, nil, manual, autoChecked) {
		t.Fatal("saved authority promoted across branches")
	}
	for name, mutate := range map[string]func(*risk.RiskEvaluation){
		"owner":                   func(e *risk.RiskEvaluation) { e.UserID = r.AccountID },
		"account":                 func(e *risk.RiskEvaluation) { e.AccountID = r.OwnerID },
		"decision":                func(e *risk.RiskEvaluation) { e.Decision = risk.Deny },
		"platform execution":      func(e *risk.RiskEvaluation) { e.PlatformExecutionAvailable = true },
		"mode":                    func(e *risk.RiskEvaluation) { e.Mode = "SHADOW" },
		"requires owner approval": func(e *risk.RiskEvaluation) { e.ApprovalRequired = true },
		"missing mandate":         func(e *risk.RiskEvaluation) { e.MandateID = nil },
		"different mandate":       func(e *risk.RiskEvaluation) { id := r.AccountID; e.MandateID = &id },
		"missing version":         func(e *risk.RiskEvaluation) { e.MandateVersion = nil },
		"different version":       func(e *risk.RiskEvaluation) { v := version + 1; e.MandateVersion = &v },
	} {
		t.Run(name, func(t *testing.T) {
			bad := autonomous
			mutate(&bad.RiskEvaluation)
			if sameSendAuthority(nil, &mandateApproval, bad, autoChecked) {
				t.Fatal("changed saved autonomous risk authority accepted")
			}
		})
	}
	for _, source := range []risk.ActionSource{"", risk.SourceUI, risk.SourceStrategy} {
		bad := autonomous
		bad.Source = source
		if sameSendAuthority(nil, &mandateApproval, bad, autoChecked) {
			t.Fatal("autonomous risk evidence lost its AI source")
		}
	}
	wrongApproval := r.AccountID
	if sameSendAuthority(&wrongApproval, nil, manual, ownerChecked) || sameSendAuthority(nil, &wrongApproval, autonomous, autoChecked) {
		t.Fatal("different approval accepted")
	}
	autoChecked.order.Request.MandateApprovalID = wrongApproval
	if sameSendAuthority(nil, &mandateApproval, autonomous, autoChecked) {
		t.Fatal("saved consent not bound to immutable request")
	}
}
