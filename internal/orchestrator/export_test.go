package orchestrator

// TruncateWithMarker re-exports the package-private
// truncateWithMarker helper so the external test package can
// exercise the tier-3 (hard-truncation) path directly without
// building a synthetic release plan large enough to drive the
// orchestrator there organically.
//
// Tests only; the canonical name stays unexported in production.
var TruncateWithMarker = truncateWithMarker
