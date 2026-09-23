package security

import (
	"errors"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12
const MaxBcryptPasswordBytes = 72

// The public contract's 8..128 characters applies when creating or changing
// passwords. bcrypt additionally limits the UTF-8 encoding to 72 bytes.
func ValidateNewPassword(password string) error {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 8 || utf8.RuneCountInString(password) > 128 {
		return errors.New("password must contain 8 to 128 characters")
	}
	if len([]byte(password)) > MaxBcryptPasswordBytes {
		return errors.New("password exceeds bcrypt's 72-byte UTF-8 limit")
	}
	return nil
}

func HashPassword(password string) (string, error) {
	if err := ValidateNewPassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	return string(hash), err
}

func VerifyPassword(hash, password string) error {
	if !utf8.ValidString(password) || len([]byte(password)) > MaxBcryptPasswordBytes {
		return errors.New("invalid password")
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// Generated once at cost 12. A real bcrypt comparison hides whether the
// account lookup found a row, without imposing new rules on legacy logins.
const dummyPasswordHash = "$2y$12$2ph7ncke/AfwsRDcAh5AY.NLUuLYmW8eEyHXTPkQbIfDBmVrslmrG"

func VerifyDummyPassword(password string) {
	if len([]byte(password)) > MaxBcryptPasswordBytes || !utf8.ValidString(password) {
		password = "dummy-password-constant"
	}
	_ = bcrypt.CompareHashAndPassword([]byte(dummyPasswordHash), []byte(password))
}
