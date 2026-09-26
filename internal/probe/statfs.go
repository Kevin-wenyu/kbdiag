package probe

// pathError names the path, as os.PathError would, so the reason says what
// could not be read.
type pathError struct {
	path string
	err  error
}

func (e *pathError) Error() string { return "statfs " + e.path + ": " + e.err.Error() }
