package backends

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/boltdb/bolt"
	"github.com/pborman/uuid"
	"golang.org/x/crypto/bcrypt"

	log "github.com/Sirupsen/logrus"
)

type User struct {
	UUID     string `json:"uuid" form:"-"`
	Username string `json:"username" form:"username"`
	Password string `json:"password" form:"password"`
	IsAdmin  bool   `json:"is_admin" form:"is_admin"`
}

func (u *User) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	err := enc.Encode(u)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func DecodeUser(user []byte) (*User, error) {
	var u *User
	buf := bytes.NewBuffer(user)
	dec := json.NewDecoder(buf)
	err := dec.Decode(&u)
	if err != nil {
		return nil, err
	}
	return u, nil
}

type AuthBackend interface {
	SetValue(key, value []byte) error
	GetValue(key []byte) ([]byte, error)

	DeleteUser(username []byte) error

	AddUser(username, password []byte, admin bool) error
	GetUser(username []byte) (*User, error)
	GetAllUsers() ([]User, error)
}

func NewBoltDBAuthBackend(db *bolt.DB, tokenBucket, userBucket []byte) *BoltAuth {
	return &BoltAuth{
		DS:          db,
		TokenBucket: []byte(tokenBucket),
		UserBucket:  []byte(userBucket),
	}
}

// UserBucketName - default name for BoltDB bucket that stores user info
const UserBucketName = "authbucket"

// TokenBucketName
const TokenBucketName = "tokenbucket"

// BoltCache - container to implement Cache instance with BoltDB backend for storage
type BoltAuth struct {
	DS          *bolt.DB
	TokenBucket []byte
	UserBucket  []byte
}

func (b *BoltAuth) AddUser(username, password []byte, admin bool) error {
	err := b.DS.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(b.UserBucket)
		if err != nil {
			return err
		}
		hashedPassword, _ := bcrypt.GenerateFromPassword(password, 10)
		u := User{
			UUID:     uuid.New(),
			Username: string(username),
			Password: string(hashedPassword),
			IsAdmin:  admin,
		}
		bts, err := u.Encode()
		if err != nil {
			log.WithFields(log.Fields{
				"error":    err.Error(),
				"username": username,
			})
			return err
		}
		err = bucket.Put(username, bts)
		if err != nil {
			return err
		}
		return nil
	})
	return err
}

func (b *BoltAuth) DeleteUser(username []byte) error {
	return b.delete(username, b.UserBucket)
}

func (b *BoltAuth) delete(key, bucket []byte) error {
	err := b.DS.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(bucket)
		if err != nil {
			return err
		}
		err = bucket.Delete(key)
		if err != nil {
			return err
		}
		return nil
	})

	return err
}

func (b *BoltAuth) GetUser(username []byte) (user *User, err error) {

	err = b.DS.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(b.UserBucket)
		if bucket == nil {
			return fmt.Errorf("Bucket %q not found!", b.UserBucket)
		}

		val := bucket.Get(username)

		// If it doesn't exist then it will return nil
		if val == nil {
			return fmt.Errorf("user not found")
		}

		user, err = DecodeUser(val)

		if err != nil {
			log.WithFields(log.Fields{
				"error":    err.Error(),
				"username": username,
			}).Error("Failed to decode user")
			return fmt.Errorf("error while getting user %q \n", username)
		}

		return nil
	})
	return
}

// GetAllUsers return all users
func (b *BoltAuth) GetAllUsers() (users []User, err error) {
	err = b.DS.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(b.UserBucket)
		if b == nil {
			// bucket doesn't exist
			return nil
		}
		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			usr, err := DecodeUser(v)
			if err != nil {
				log.WithFields(log.Fields{
					"error": err.Error(),
					"json":  v,
				}).Warning("Failed to deserialize bytes to user.")
			} else {
				users = append(users, *usr)
			}
		}
		return nil
	})
	return
}

func (b *BoltAuth) SetValue(key, value []byte) error {
	err := b.DS.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(b.TokenBucket)
		if err != nil {
			return err
		}
		err = bucket.Put(key, value)
		if err != nil {
			return err
		}
		return nil
	})

	return err
}

func (b *BoltAuth) GetValue(key []byte) (value []byte, err error) {

	err = b.DS.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(b.TokenBucket)
		if bucket == nil {
			return fmt.Errorf("Bucket %q not found!", b.TokenBucket)
		}
		// "Byte slices returned from Bolt are only valid during a transaction."
		var buffer bytes.Buffer
		val := bucket.Get(key)

		// If it doesn't exist then it will return nil
		if val == nil {
			return fmt.Errorf("key %q not found \n", key)
		}

		buffer.Write(val)
		value = buffer.Bytes()
		return nil
	})

	return
}
