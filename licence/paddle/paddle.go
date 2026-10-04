// Package paddle is what the dispenser knows about Paddle's own words: its
// adjustments and when they mean the money went back. The pretend Paddle
// of the local dispenser and the client for Paddle's API both read a sale
// through it, so the two cannot disagree about a refund.
//
// See https://developer.paddle.com/webhooks/adjustments/adjustment-created
package paddle

import "time"

// What an adjustment does, Paddle's action field.
const (
	Credit                   = "credit"
	CreditReverse            = "credit_reverse"
	Refund                   = "refund"
	Chargeback               = "chargeback"
	ChargebackReverse        = "chargeback_reverse"
	ChargebackWarning        = "chargeback_warning"
	ChargebackWarningReverse = "chargeback_warning_reverse"
)

// Where an adjustment stands, Paddle's status field. A refund starts
// pending approval on a live account and becomes approved or rejected.
const (
	PendingApproval = "pending_approval"
	Approved        = "approved"
	Rejected        = "rejected"
	Reversed        = "reversed"
)

// Adjustment is one of Paddle's adjustments to a transaction.
type Adjustment struct {
	ID            string
	TransactionID string
	Action        string
	Status        string
	At            time.Time
}

// TakenBack says whether Paddle has the money of a sale back: a refund of
// it was approved, or a chargeback stands that was not reversed. A refund
// still waiting for approval or rejected, a chargeback warning and a
// credit do not count.
//
// A reversal shows twice at Paddle: as an adjustment of its own, and as
// the chargeback's status turning to reversed. Either can be read before
// the other, so a reversal that no chargeback shows yet is taken off one
// that still stands. It goes by counts and not by order, so adjustments
// read in any order give the same answer.
func TakenBack(adjs []Adjustment) bool {
	standing, reversedShown, reversals := 0, 0, 0
	for _, a := range adjs {
		switch a.Action {
		case Refund:
			if a.Status == Approved {
				return true
			}
		case Chargeback:
			switch a.Status {
			case Reversed:
				reversedShown++
			case Rejected:
			default:
				standing++
			}
		case ChargebackReverse:
			reversals++
		}
	}
	return standing-max(reversals-reversedShown, 0) > 0
}
