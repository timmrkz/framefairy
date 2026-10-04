package paddle

import (
	"math/rand/v2"
	"testing"
)

func adj(action, status string) Adjustment {
	return Adjustment{Action: action, Status: status}
}

func TestTakenBack(t *testing.T) {
	cases := []struct {
		name string
		adjs []Adjustment
		want bool
	}{
		{"nothing", nil, false},
		{"refund waiting", []Adjustment{adj(Refund, PendingApproval)}, false},
		{"refund approved", []Adjustment{adj(Refund, Approved)}, true},
		{"refund rejected", []Adjustment{adj(Refund, Rejected)}, false},
		{"chargeback", []Adjustment{adj(Chargeback, Approved)}, true},
		{"chargeback reversed", []Adjustment{adj(Chargeback, Approved), adj(ChargebackReverse, Approved)}, false},
		{"reversal read first", []Adjustment{adj(ChargebackReverse, Approved), adj(Chargeback, Approved)}, false},
		{"second chargeback", []Adjustment{adj(Chargeback, Approved), adj(ChargebackReverse, Approved), adj(Chargeback, Approved)}, true},
		{"warning", []Adjustment{adj(ChargebackWarning, Approved)}, false},
		{"warning reversed", []Adjustment{adj(ChargebackWarning, Approved), adj(ChargebackWarningReverse, Approved)}, false},
		{"credit", []Adjustment{adj(Credit, Approved)}, false},
		{"refund after a reversed chargeback", []Adjustment{adj(Chargeback, Approved), adj(ChargebackReverse, Approved), adj(Refund, Approved)}, true},
		{"unknown action", []Adjustment{adj("something_new", Approved)}, false},
		{"chargeback shown reversed, reversal not yet", []Adjustment{adj(Chargeback, Reversed)}, false},
		{"chargeback shown reversed, and its reversal", []Adjustment{adj(Chargeback, Reversed), adj(ChargebackReverse, Approved)}, false},
		{"a new chargeback after a reversed one", []Adjustment{adj(Chargeback, Reversed), adj(ChargebackReverse, Approved), adj(Chargeback, Approved)}, true},
		{"two chargebacks, one reversal not shown yet", []Adjustment{adj(Chargeback, Approved), adj(Chargeback, Approved), adj(ChargebackReverse, Approved)}, true},
		{"two chargebacks, both reversed", []Adjustment{adj(Chargeback, Reversed), adj(Chargeback, Approved), adj(ChargebackReverse, Approved), adj(ChargebackReverse, Approved)}, false},
	}
	for _, c := range cases {
		if got := TakenBack(c.adjs); got != c.want {
			t.Errorf("%s: TakenBack %v, want %v", c.name, got, c.want)
		}
	}
}

// The order Paddle's adjustments are read in never changes the answer.
func TestTakenBackIgnoresOrder(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	actions := []string{Refund, Chargeback, ChargebackReverse, ChargebackWarning, Credit}
	statuses := []string{PendingApproval, Approved, Rejected, Reversed}
	for range 2000 {
		var adjs []Adjustment
		for range r.IntN(6) {
			adjs = append(adjs, adj(actions[r.IntN(len(actions))], statuses[r.IntN(len(statuses))]))
		}
		want := TakenBack(adjs)
		for range 5 {
			r.Shuffle(len(adjs), func(i, j int) { adjs[i], adjs[j] = adjs[j], adjs[i] })
			if TakenBack(adjs) != want {
				t.Fatalf("%v read in another order gives another answer", adjs)
			}
		}
	}
}
