package opsstatus

import (
	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
)

// BuildDatasetSection reports what is in the dataset directory, so the question
// "is there any data on this box?" is answerable from the browser.
//
// It is deliberately the same view the importer takes — same directory, same
// non-recursive file rule — so a count shown here is a count import will read.
func BuildDatasetSection() map[string]any {
	inv := dataset.Inspect(dataset.Dir())
	return map[string]any{
		"path":            inv.Path,
		"exists":          inv.Exists,
		"readable":        inv.Readable,
		"match_files":     inv.MatchFiles,
		"bytes":           inv.Bytes,
		"newest_file":     inv.NewestFile,
		"newest_modified": inv.NewestModified,
		"empty":           inv.IsEmpty(),
		"error":           inv.Error,
		"env_var":         dataset.DirEnvVar,
	}
}
