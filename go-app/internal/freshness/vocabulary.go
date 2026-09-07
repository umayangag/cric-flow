// Package freshness holds the words this system spells one freshness verdict with.
//
// There is exactly one verdict, and it is ml-service's (H-11): a live prediction is
// refused when the loaded rating state is older than `ml.ratings_max_age_days`. Nothing
// in this package re-decides it and nothing here holds a threshold — go-app reads the
// limit off the verdict it was given. What lives here is the vocabulary that crosses the
// service boundary: the badge every surface renders, and the code the refusal carries.
//
// It is its own package, with no dependencies, because three components have to agree on
// these literals (H-24) and the generated contract is built from a package the pipeline
// registry can import. `db_freshness` reading *stale* while H-11 read *fresh* on the same
// box is what two private vocabularies cost (PRODUCT_ROADMAP § 2.1 gap (4), P2-1).
package freshness

// The served verdict's vocabulary — how H-11's answer is spelled on the wire.
const (
	// Fresh: the loaded ratings are inside the limit, so a prediction is served.
	Fresh = "fresh"
	// Stale: something is loaded and past the limit, so a live prediction is refused with
	// RatingsStaleCode.
	Stale = "stale"
	// NotLoaded: no rating state at all — refused for the other reason, and with no state
	// there is no age to report. Not the same thing as stale: the remedy is a reload, not
	// a retrain, and the deleted buckets could not tell them apart.
	NotLoaded = "not_loaded"
	// Unknown: ml-service did not answer, so this system does not know. It says so rather
	// than reporting a verdict it never received (§8.7).
	Unknown = "unknown"
)

// Statuses is the served-verdict vocabulary, in badge order.
func Statuses() []string { return []string{Fresh, Stale, NotLoaded, Unknown} }

// The retrain-due vocabulary — whether the database holds matches the served run never
// saw (the B-2 state a green pipeline once hid).
const (
	// UpToDate: the served ratings run through the database's latest match.
	UpToDate = "up_to_date"
	// RetrainDue: matches have been imported since the served run was built.
	RetrainDue = "retrain_due"
)

// RetrainStatuses is the retrain-due vocabulary. `unknown` is the same word the served
// verdict uses, and means the same thing: one of the two facts is missing, so the
// comparison was not made rather than defaulted either way.
func RetrainStatuses() []string { return []string{UpToDate, RetrainDue, Unknown} }

// RatingsStaleCode is the code ml-service raises a live prediction's refusal with (H-11)
// and reports on its own verdict. go-app never matches on it — it relays ml-service's
// error body untouched — but the literal reaches the Lab's refusal notice and the Ops
// badge, so it is declared here for the contract and asserted from ml-service, where it
// originates, and the frontend, which renders it.
const RatingsStaleCode = "RATINGS_STALE"
