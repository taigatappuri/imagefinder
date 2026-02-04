package security

import (
	"encoding/base64"
	"errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"
)

type KMSEncryptor struct {
	Client *kms.KMS
	KeyID  string
}

func NewKMSEncryptor(keyID string, region string) (*KMSEncryptor, error) {
	if keyID == "" {
		return nil, errors.New("KMS キー ID が未設定です")
	}
	if region == "" {
		return nil, errors.New("AWS リージョンが未設定です")
	}
	sess, err := session.NewSession(&aws.Config{Region: aws.String(region)})
	if err != nil {
		return nil, err
	}
	return &KMSEncryptor{Client: kms.New(sess), KeyID: keyID}, nil
}

func (k *KMSEncryptor) Encrypt(plaintext string) (string, error) {
	output, err := k.Client.Encrypt(&kms.EncryptInput{
		KeyId:     aws.String(k.KeyID),
		Plaintext: []byte(plaintext),
	})
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(output.CiphertextBlob), nil
}

func (k *KMSEncryptor) Decrypt(encoded string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	output, err := k.Client.Decrypt(&kms.DecryptInput{
		CiphertextBlob: ciphertext,
	})
	if err != nil {
		return "", err
	}
	return string(output.Plaintext), nil
}
