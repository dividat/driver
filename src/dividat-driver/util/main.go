package util

// return pointer to value, useful when handling const
func PointerTo[T any](val T) *T {
	return &val
}
