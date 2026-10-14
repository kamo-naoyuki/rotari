//go:build !linux

package bugreport

func filesystemType(string) string {
	return "unknown (not detected on this OS)"
}
