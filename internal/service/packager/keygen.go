package packager

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// generatedPrivateKeyFileMode is secure file mode for signing private key output.
const generatedPrivateKeyFileMode = 0o600

// errSigningKeyAlreadyExists guards against accidental key overwrite.
var errSigningKeyAlreadyExists = errors.New("signing key already exists; refusing to overwrite")

// SigningKeyMaterial describes generated key information.
type SigningKeyMaterial struct {
	// PrivateKeyPath is filesystem path where private key PEM was saved.
	PrivateKeyPath string
	// KeyID is recommended manifest signing key identifier.
	KeyID string
	// PublicKeyBase64 is trusted public key representation for updater embedding.
	PublicKeyBase64 string
}

// SuggestedKeyID returns UTC month-based key identifier (YYYY-MM).
func SuggestedKeyID() string {
	return time.Now().UTC().Format("2006-01")
}

// GenerateSigningKey creates Ed25519 keypair and writes private key in PKCS8 PEM.
func GenerateSigningKey(privateKeyPath, keyID string, force bool) (*SigningKeyMaterial, error) {
	if privateKeyPath == "" {
		return nil, errPrivateKeyPathRequired
	}

	if keyID == "" {
		keyID = SuggestedKeyID()
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	privateKeyPKCS8, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyPKCS8,
	})
	if privateKeyPEM == nil {
		return nil, errPrivateKeyPEMBlockNotFound
	}

	path := filepath.Clean(privateKeyPath)

	err = writePrivateKey(path, privateKeyPEM, force)
	if err != nil {
		return nil, err
	}

	return &SigningKeyMaterial{
		PrivateKeyPath:  path,
		KeyID:           keyID,
		PublicKeyBase64: base64.StdEncoding.EncodeToString(publicKey),
	}, nil
}

// writePrivateKey writes PKCS8 PEM private key with overwrite policy enforcement.
func writePrivateKey(path string, payload []byte, force bool) error {
	flags := os.O_WRONLY | os.O_CREATE
	if force {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}

	file, err := os.OpenFile(path, flags, generatedPrivateKeyFileMode)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return errSigningKeyAlreadyExists
		}

		return fmt.Errorf("open private key output: %w", err)
	}
	defer func() { _ = file.Close() }()

	err = file.Chmod(generatedPrivateKeyFileMode)
	if err != nil {
		return fmt.Errorf("set private key permissions: %w", err)
	}

	_, err = file.Write(payload)
	if err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	err = file.Sync()
	if err != nil {
		return fmt.Errorf("sync private key: %w", err)
	}

	return nil
}
