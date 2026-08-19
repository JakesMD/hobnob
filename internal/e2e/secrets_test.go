package e2e

import "testing"

// Most secret-masking behavior is proven right next to the feature that
// produces it — set: in set_test.go, get: in get_test.go, with: in
// call_test.go. This file covers the masking behaviors that cut across
// features rather than belonging to any one of them.

func TestE2E_Secrets_JSONEscapedFormAlsoMasked(t *testing.T) {
	// given a secret value containing a double quote, when it's embedded as
	// a leaf of a set: JSON literal and the resulting object is referenced
	// in a run: command, then the *escaped* form (json.Marshal doubles the
	// quote as \") is masked too, not just the raw form (why: a secret
	// flowing through a JSON literal is marshaled once — its escaped text in
	// the displayed command can differ byte-for-byte from the raw secret, and
	// both must be caught)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - TOKEN:
		              value: 'ab"cd'
		              secret: true
		          - OBJ:
		              token: "{{.TOKEN}}"
		      - run: ": {{.OBJ}}"
	`, "t")
	res.OK(t)
	res.Masked(t, `ab"cd`)
}

func TestE2E_Secrets_AllOccurrencesInOneCommandMasked(t *testing.T) {
	// given a secret referenced twice in the same run: command, when the
	// command is displayed, then every occurrence is masked, not just the
	// first
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - PASS:
		              value: hunter2
		              secret: true
		      - run: ": {{.PASS}} {{.PASS}}"
	`, "t")
	res.OK(t)
	res.Masked(t, "hunter2")
	// Belt and braces beyond .Masked's single-absence check: confirm there
	// isn't a first-occurrence-only bug by counting mask markers.
	count := 0
	for i := 0; i+4 <= len(res.Combined); i++ {
		if res.Combined[i:i+4] == "****" {
			count++
		}
	}
	if count < 2 {
		t.Errorf("expected at least 2 mask markers (one per occurrence), got %d in:\n%s", count, res.Combined)
	}
}

func TestE2E_Secrets_NonSecretVarWithSameValueAsUnrelatedSecretNotAssumedSafe(t *testing.T) {
	// given a secret var and a coincidentally-identical non-secret var, when
	// the non-secret one is echoed, then it's masked too — masking matches
	// on the value stored under a secret key, not on which var name a
	// template happens to reference (why: this is a documented tradeoff, not
	// a bug — see runner.maskSecrets)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - SECRET_ONE:
		              value: shared-value
		              secret: true
		          - PLAIN_TWO: shared-value
		      - run: ": {{.PLAIN_TWO}}"
	`, "t")
	res.OK(t)
	res.Masked(t, "shared-value")
}
