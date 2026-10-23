package envelope

// Error categories: the kind of failure, which says whether re-running could
// help. The set is open; a category with no constant here is still valid.
const (
	// CategoryUsage is an invocation that was wrong. Nothing was written.
	CategoryUsage = "usage"
	// CategoryAuth is a missing or rejected credential. Re-running changes
	// nothing until the environment does.
	CategoryAuth = "auth"
	// CategoryInput is an input that could not be read or does not exist: an
	// issue the forge does not have, a request it refused as invalid, or a
	// forge that cannot be told.
	CategoryInput = "input"
	// CategoryConflict is a write the state of the forge does not allow, such
	// as a pull request that cannot be merged.
	CategoryConflict = "conflict"
	// CategoryAPI is a provider or transport failure, a rate limit included.
	// It is retryable.
	CategoryAPI = "api"
	// CategoryAborted is a call that was cancelled or outlived its timeout.
	// It is retryable.
	CategoryAborted = "aborted"
	// CategoryInternal is a bug in the command.
	CategoryInternal = "internal"
)

// RetryableFor reports whether an error of the given category can succeed if
// re-run unchanged. Only "api" and "aborted" are retryable.
func RetryableFor(category string) bool {
	return category == CategoryAPI || category == CategoryAborted
}

// FixHint is a machine-usable remedy for a failure.
type FixHint struct {
	Flag  string   `json:"flag,omitempty" description:"the flag to change"`
	Env   []string `json:"env,omitempty" description:"the environment variables, one of which supplies what is missing"`
	Valid []string `json:"valid,omitempty" description:"the values the flag accepts here"`
}

// ErrorInfo says what failed and where. It is present exactly when ok is
// false. It is an error, so a caller can return it as it is.
type ErrorInfo struct {
	// Stage is the step that failed: usage, forge (the forge or its client
	// could not be set up), read, write, or schema.
	Stage string `json:"stage" description:"the step that failed"`
	// Category says whether re-running could help.
	Category  string   `json:"category" description:"the kind of failure, which says whether re-running could help; an open set"`
	Message   string   `json:"message" description:"what failed, in words"`
	Retryable bool     `json:"retryable" description:"whether re-running the identical command could succeed"`
	Resumable bool     `json:"resumable" description:"whether re-running the same input continues the work rather than starting it over"`
	FixHint   *FixHint `json:"fix_hint,omitempty" description:"a mechanical remedy, where one is known"`
	err       error
}

func (e *ErrorInfo) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Unwrap returns the failure the command raised this from. It is set only in
// the process that ran the command; an ErrorInfo decoded from JSON has none.
func (e *ErrorInfo) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// WithCause records the failure e reports, for Unwrap, and returns e.
func (e *ErrorInfo) WithCause(err error) *ErrorInfo {
	e.err = err
	return e
}
