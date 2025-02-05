// Copyright (C) 2021 SeanTolstoyevski -  mailto:seantolstoyevski@protonmail.com
// The source code of this project is licensed under the MIT license.
// You can find the license on the repo's main folder.
// Provided without warranty of any kind.

//Package pengine low-level APIs for paket.
//
// Before using it, you need to create a file with the cmd tool. (If you are not creating a new tool or API).
//
// Users do not need functions and structures other than New and Paket methods.
//
//Other exported functions and variables are for the cmd tool.
// If you only want to read the package created with the cmd tool,
// you can create a new package method with New().
package pengine

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

var (
	// If there is no data in the map sent to New, the functions you use will return this error.
	ErrMinimumMapValue = errors.New("map cannot be less than 1 in length")
)

// type declaration for map values.
type Values struct {
	// start position
	StartPos int

	// end position
	EndPos int

	// length of the original file.
	OriginalLenght int

	// length of the encrypted data.
	EncryptLenght int

	// Hash of the original file.
	HashOriginal string

	// Hash of encrypted data.
	HashEncrypt string
}

// type definition for the Paket.
//
// Paket reads the requested file through this map.
//
// string refers to the file name, values refers to information about the file (length, sha value etc.).
type Datas map[string]Values

// CreateRandomBytes generates random bytes of the given size.
//The maximum value should be 32 and the minimum value should be 16.
//
//Used to generate a random key if the user has not specified a key. (for cmd tool)
//
// Returns error for the wrong size or  creating bytes.
func CreateRandomBytes(l uint8) ([]byte, error) {
	if l < 16 || l > 32 {
		return nil, errors.New("minimum value for l is 16, maximum value for l is 32")
	}
	res := make([]byte, l)
	_, err := rand.Read(res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// Encrypt encrypts the data using the key.
//
// Uses the CFB mode.
//
// Key must be 16, 24 or 32 size.
// Otherwise, the cypher module returns an error.
//
// You can compare the data sended  to the function with the output data. It might be a good idea to make sure it's working properly.
//
//If everything is working correctly, it returns an encrypted bytes and nil error.
func Encrypt(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	ciphertext := make([]byte, aes.BlockSize+len(data))
	v := ciphertext[:aes.BlockSize]

	_, rerr := io.ReadFull(rand.Reader, v)
	if rerr != nil {
		return nil, rerr
	}

	s := cipher.NewCFBEncrypter(block, v)
	s.XORKeyStream(ciphertext[aes.BlockSize:], data)
	return ciphertext, nil
}

// Decrypt decrypts the encrypted data with the key.
//
// Uses the CFB mode.
//
// It doesn't matter whether you have the correct key or not. It decrypts data with the key given under any condition.
// So you should compare it with the original data with a suitable hash function (see sha256, sha512 module...).
// Otherwise, you can't be sure it is returning the correct data.
//
// If everything is working correctly, it returns  decrypted bytes and nil error.
func Decrypt(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	iv := data[:aes.BlockSize]
	data = data[aes.BlockSize:]
	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(data, data)
	return data, nil
}

// Paket that keeps the information of the file to be read.
// It should be created with New.
type Paket struct {
	// Key value for reading the file's data.
	// As a warning, you shouldn't just create a plaintext key.
	Key []byte
	// name of the file from which the data was taken.
	// Required for various functions.
	paketFileName string
	// Map value that keep the information of files in Paket.
	// It must be at least 1 length.
	// Otherwise, panic occurs at runtime.
	//
	// Usually created by the cmd tool.
	Table Datas

	//non-exported value created for access the file.
	// This value is opened by New with filename parameter.
	// file released with the Close function.
	file *os.File

	// Used to prevent conflicts in GetFile. For files requested at the same time.
	globMut sync.Mutex
}

// New Creates a new Package method.
// This method should be used to read the files.
//
// key parameter refers to the encryption key. It must be 16, 24 or 32 length. Returns nil and error for keys of incorrect length.
//
// Panic occurs if the specified file does not exist.
//
// table parameter is defined in go file created by the cmd tool.
// There must be a minimum of 1 file in the table.
//
// After getting all the data you need, should be terminated with  Close.
func New(key []byte, paketFileName string, table Datas) (*Paket, error) {
	l := len(key)
	if l == 16 || l == 24 || l == 32 {
		if !Exists(paketFileName) {
			panic(paketFileName + " paket not found.")
		}

		f, err := os.Open(paketFileName)
		if err != nil {
			return nil, err
		}

		fInfo, ferr := f.Stat()
		if ferr != nil {
			return nil, err
		}

		if fInfo.Size() > 0 {
			return &Paket{file: f, Table: table, Key: key, paketFileName: paketFileName}, nil
		}
		perr := "there is no data in the file: " + f.Name()
		panic(perr)
	}
	return nil, errors.New("key must be 16, 24 or 32 length")
}

// GetFile Returns the content of the requested file.
//
// If the file cannot be found in the map and the length cannot be read, a panic occurs.
//
// All errors except these errors return with error.
//
// If decrypt is true, it is decrypted. If not, encrypted bytes are returned.
//
// If value of shaControl is true, the hash of the decrypted data is compared with hash of the original file.
//
// If decrypt is false and shaControl is true, the hash of the encrypted file in the table is compared with the encrypted hash of the read file.
//
// If the hash comparison is true, the second value is set to true.
//
// If hashControl is false, checks are skipped. Returns False.
//
// Both values do not have to be true. However, it may be good to generate a control mechanism like hash with your own work.
// The decrypt (bool) value has been added for convenience. As a recommendation,
// it is better to pass both values to true to this function.
func (p *Paket) GetFile(filename string, decrypt, shaControl bool) ([]byte, bool, error) {
	file, found := p.Table[filename]
	if !found {
		return nil, false, errors.New("File not found on map: " + filename)
	}

	p.globMut.Lock()
	defer p.globMut.Unlock()

	// We need the length of the encrypted data to be able to load to memory the file
	length := file.EncryptLenght
	// The position where our new file starts. Should be calculated based on the encrypted file length rather than the original file
	start := file.StartPos

	content := make([]byte, length)

	// We go to the position of file
	_, err := p.file.Seek(int64(start), 0)
	if err != nil {
		return nil, false, err
	}
	// We read it to the position we want. So in this case, up to the position  where the encrypted data ends. We Alocated the *content* variable
	_, rerr := p.file.Read(content)
	if rerr != nil {
		return nil, false, rerr
	}
	switch decrypt {
	case true:
		decryptedData, err := Decrypt(p.Key, content)
		if err != nil {
			return nil, false, err
		}
		if shaControl {
			decryptedHash := []byte(fmt.Sprintf("%x", sha256.Sum256(decryptedData)))
			encryptedHash := []byte(file.HashEncrypt)
			return decryptedData, bytes.Equal(decryptedHash, encryptedHash), nil
		}
		return decryptedData, false, nil
	case false:
		if shaControl {
			forgSha := []byte(file.HashEncrypt)
			corgSha := []byte(fmt.Sprintf("%x", sha256.Sum256(content)))
			return content, bytes.Equal(corgSha, forgSha), nil
		}
		return content, false, nil
	default:
		return content, false, nil
	}
}

// GetGoroutineSafe created to securely retrieve data when using with multiple goroutines.
// In any case, it only returns decrypted data.
//
// It does not do any hash checking.
func (p *Paket) GetGoroutineSafe(name string) ([]byte, error) {
	file, found := p.Table[name]
	if !found {
		return nil, errors.New("File not found on map: " + name)
	}
	length := file.EncryptLenght
	encryptedLenght, _ := p.GetLen()
	if length > encryptedLenght[1] {
		return nil, errors.New("more length than file size")
	}
	start := file.StartPos

	f, err := os.Open(p.paketFileName)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.Seek(int64(start), 0); err != nil {
		return nil, err
	}
	content := make([]byte, length)
	if _, err := f.Read(content); err != nil {
		return nil, err
	}
	decryptedData, err := Decrypt(p.Key, content)
	if err != nil {
		content = nil // I don't understand what the gc of Go does sometimes. A guarantee
		return nil, err
	}

	content = nil // I don't understand what the gc of Go does sometimes. A guarantee
	return decryptedData, nil
}

// GetLen Returns the original and encrypted lengths of all files contained in Paket.
// 0 index refers to the original, 1  index to the encrypted data.
// In the meantime, no control is made. The same will return as the values are written into the table.
//
// Normally values should be in bytes.
//
// returns an error if length is less than 1(see ErrMinimumMapValue). This case, other  things are 0.
func (p *Paket) GetLen() ([2]int, error) {
	values := [2]int{}
	if len(p.Table) < 1 {
		return values, ErrMinimumMapValue
	}
	for _, value := range p.Table {
		values[0] += value.OriginalLenght
		values[1] += value.EncryptLenght
	}
	return values, nil
}

// Close Closes the opened file (see Paket.file (non-exported)).
//
// Use this function when all your transactions are done (so you shouldn't use it with defer or something like that).
// Otherwise, you must create a new Paket method.
//
// When you call Close, you cannot access the Package again.
//
// Returns error for unsuccessful events.
func (p *Paket) Close() error {
	err := p.file.Close()
	if err != nil {
		return nil
	}
	return err
}

// a guarantee about the existence of file.
//
// Source: https://stackoverflow.com/a/12527546/13431469
// Thanks to SO user.
func Exists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}
