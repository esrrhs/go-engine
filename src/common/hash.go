package common

import (
	"crypto/md5"
	"encoding/hex"
	"hash/crc32"
)

func GetMd5String(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func GetCrc32String(s string) string {
	hash := crc32.New(crc32.IEEETable)
	hash.Write([]byte(s))
	hashInBytes := hash.Sum(nil)[:]
	return hex.EncodeToString(hashInBytes)
}

func GetCrc32(data []byte) string {
	hash := crc32.New(crc32.IEEETable)
	hash.Write(data)
	hashInBytes := hash.Sum(nil)[:]
	return hex.EncodeToString(hashInBytes)
}
