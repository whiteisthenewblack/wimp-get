// +build windows

package platform

import(
	"regexp"
)

func SanitiseFilename(filename string) (newName string, e error) {
	r, e := regexp.Compile("[\\?<>:/\"\\\\|\\*]")
	if e != nil {
		return
	}

	newName = r.ReplaceAllString(filename, "")
	return
}

func DirOf(filename string) (dirname string, e error) {
	r, e := regexp.Compile(`[^\ ]+$`)
	if e != nil {
		return
	}

	dirname = r.ReplaceAllString(filename, "")
	return
}
