package execution

import (
	"testing"
	"time"
)

func TestScheduledProposalIdentityIsStableAndSlotScoped(t *testing.T) {
	bucket, mandate := "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	slot := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	first := scheduledProposalClientID(bucket, mandate, 1, slot)
	if !validUUID(first) || first[14] != '8' || scheduledProposalClientID(bucket, mandate, 1, slot) != first {
		t.Fatal("scheduled identity is not deterministic UUIDv8", first)
	}
	for name, candidate := range map[string]string{
		"bucket":  scheduledProposalClientID(mandate, mandate, 1, slot),
		"mandate": scheduledProposalClientID(bucket, bucket, 1, slot),
		"version": scheduledProposalClientID(bucket, mandate, 2, slot),
		"slot":    scheduledProposalClientID(bucket, mandate, 1, slot.Add(30*time.Minute)),
	} {
		if candidate == first {
			t.Fatal("distinct source reused client identity", name)
		}
	}
}

func TestScheduledProposalDeadlineRejectsNoncanonicalFutureAndBackfill(t *testing.T) {
	anchor := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	now := anchor.Add(time.Minute)
	for name, input := range map[string]struct {
		slot, now, anchor time.Time
		interval          int
	}{
		"zero slot":           {time.Time{}, now, anchor, 30},
		"zero current time":   {anchor, time.Time{}, anchor, 30},
		"zero effective time": {anchor, now, time.Time{}, 30},
		"non-UTC slot":        {anchor.In(time.FixedZone("offset", 3600)), now, anchor, 30},
		"subsecond slot":      {anchor.Add(time.Nanosecond), now, anchor, 30},
		"off grid":            {anchor.Add(time.Second), now, anchor, 30},
		"future slot":         {anchor.Add(30 * time.Minute), now, anchor, 30},
		"before effective":    {anchor.Add(-30 * time.Minute), now, anchor, 30},
		"expired window":      {anchor, anchor.Add(2 * time.Minute), anchor, 30},
		"backfill window":     {anchor, anchor.Add(30 * time.Minute), anchor, 30},
		"short cadence":       {anchor, now, anchor, 29},
		"long cadence":        {anchor, now, anchor, 1441},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := scheduledProposalDeadline(input.slot, input.now, input.anchor, input.interval); err == nil {
				t.Fatal("invalid source timing accepted")
			}
		})
	}
}

func TestScheduledProposalDeadlineCapsAllAuthorityAndRoundsEligibilityUp(t *testing.T) {
	anchor := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	now := anchor.Add(time.Minute)
	cap := now.Add(15 * time.Second)
	until, err := scheduledProposalDeadline(anchor, now, anchor, 30, anchor.Add(time.Hour), cap, now.Add(30*time.Second))
	if err != nil || !until.Equal(cap) {
		t.Fatal("source outlives shortest authority", until, err)
	}
	if _, err := scheduledProposalDeadline(anchor, now, anchor, 30, now); err == nil {
		t.Fatal("expired authority accepted")
	}
	effective := anchor.Add(100 * time.Millisecond)
	if _, err := scheduledProposalDeadline(anchor, now, effective, 30); err == nil {
		t.Fatal("slot rounded below immutable mandate eligibility")
	}
	first := anchor.Add(time.Second)
	if until, err := scheduledProposalDeadline(first, now, effective, 30); err != nil || !until.Equal(first.Add(2*time.Minute)) {
		t.Fatal("ceiling-anchored first slot rejected", until, err)
	}
}

func TestScheduledProposalOneConsentCanCoverLaterValidSlots(t *testing.T) {
	anchor := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	// A two-hour immutable consent can cover later real cadence slots; replacing
	// consent is not required and cannot shift their original version anchor.
	consentUntil := anchor.Add(2 * time.Hour)
	for _, elapsed := range []time.Duration{0, 30 * time.Minute, time.Hour, 90 * time.Minute} {
		slot := anchor.Add(elapsed)
		until, err := scheduledProposalDeadline(slot, slot.Add(time.Minute), anchor, 30, consentUntil)
		if err != nil || !until.Equal(slot.Add(2*time.Minute)) {
			t.Fatal("same consent cannot cover its later valid slot", slot, until, err)
		}
	}
	if _, err := scheduledProposalDeadline(consentUntil, consentUntil, anchor, 30, consentUntil); err == nil {
		t.Fatal("slot renewed expired consent")
	}
}
