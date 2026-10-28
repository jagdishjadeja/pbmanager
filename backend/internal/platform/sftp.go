package platform

import (
	"errors"
	"regexp"
	"strings"
)

var sftpNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]{2,23}$`)

func ValidateSFTPUsername(username string) error {
	if !sftpNamePattern.MatchString(username) {
		return errors.New("use 3–24 lowercase letters and numbers, starting with a letter")
	}
	for _, name := range []string{"root", "admin", "caddy", "pbmanager", "ubuntu", "debian", "nobody", "daemon", "wwwdata"} {
		if username == name {
			return errors.New("this username is reserved")
		}
	}
	return nil
}
func ValidateSFTPPassword(password string) error {
	if len(password) < 12 || len(password) > 128 || strings.ContainsAny(password, "\r\n:\x00") {
		return errors.New("use a password of 12–128 characters without colons or line breaks")
	}
	return nil
}
