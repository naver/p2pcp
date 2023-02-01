// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"bytes"
	"encoding/hex"
	"io"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/DataDog/zstd"
)

// randbytes creates a stream of non-crypto quality random bytes
type randbytes struct {
	rand.Source
}

// Read satisfies io.Reader
func (r *randbytes) Read(p []byte) (n int, err error) {
	todo := len(p)
	offset := 0
	for {
		val := int64(r.Int63())
		for i := 0; i < 8; i++ {
			p[offset] = byte(val)
			todo--
			if todo == 0 {
				return len(p), nil
			}
			offset++
			val >>= 8
		}
	}
}

// NewRandBytesFrom creates a new reader from your own rand.Source
func NewRandBytesFrom(src rand.Source) io.Reader {
	return &randbytes{src}
}

// NewRandBytes creates a new random reader with a time source.
func NewRandBytes() io.Reader {
	return NewRandBytesFrom(rand.NewSource(time.Now().UnixNano()))
}

func GenerateRandData(length int) []byte {
	buffer := make([]byte, length)

	r := NewRandBytes()
	r.Read(buffer)

	return buffer
}

func init() {
}

func writeFile(fname string, data []byte) (err error) {
	var file *os.File
	file, err = os.Create(fname)
	if err != nil {
		return
	}
	defer file.Close()

	file.Write(data)
	return
}

func initTestEncoding() {
	EncodingPriorMap = map[string]int{
		EncodingTypeNone: 9,
		"c":              4,
		"b":              2,
		"d":              6,
		"a":              1,
	}
}

func test(t *testing.T, a interface{}, e interface{}) {
	if a != e {
		t.Fatalf("results are different: %v (actual) != %v (expected)", a, e)
	}
}

func TestGetPreferredEncodingType(t *testing.T) {
	initTestEncoding()

	test(t, GetPreferredEncodingType(nil), EncodingTypeNone)
	test(t, GetPreferredEncodingType([]string{""}), EncodingTypeNone)
	test(t, GetPreferredEncodingType([]string{" "}), EncodingTypeNone)
	test(t, GetPreferredEncodingType([]string{" , "}), EncodingTypeNone)
	test(t, GetPreferredEncodingType([]string{"a , b"}), "a")
	test(t, GetPreferredEncodingType([]string{" a , b, c "}), "a")
	test(t, GetPreferredEncodingType([]string{"b, c"}), "b")
	test(t, GetPreferredEncodingType([]string{"b, c, d"}), "b")
	test(t, GetPreferredEncodingType([]string{"c, " + EncodingTypeNone}), "c")
	test(t, GetPreferredEncodingType([]string{EncodingTypeNone}), EncodingTypeNone)
}

func TestIsAvailableEncoding(t *testing.T) {
	initTestEncoding()

	var name string

	name = "a"
	if IsAvailableEncoding(&name) != true || name != "a" {
		t.Fatalf("results are different")
	}

	name = "b"
	if IsAvailableEncoding(&name) != true || name != "b" {
		t.Fatalf("results are different")
	}

	name = "c"
	if IsAvailableEncoding(&name) != true || name != "c" {
		t.Fatalf("results are different")
	}

	name = "d"
	if IsAvailableEncoding(&name) != true || name != "d" {
		t.Fatalf("results are different")
	}

	name = EncodingTypeNone
	if IsAvailableEncoding(&name) != true || name != EncodingTypeNone {
		t.Fatalf("results are different")
	}

	name = ""
	if IsAvailableEncoding(&name) != true || name != EncodingTypeNone {
		t.Fatalf("results are different")
	}

	if IsAvailableEncoding(nil) != false {
		t.Fatalf("results are different")
	}
}

func TestInitEncoding(t *testing.T) {
	initTestEncoding()

	test(t, EncodingPriorMap["a"], 1)
	test(t, EncodingPriorMap["b"], 2)
	test(t, EncodingPriorMap["c"], 4)
	test(t, EncodingPriorMap["d"], 6)
	test(t, EncodingPriorMap[EncodingTypeNone], 9)
}

func TestLength(t *testing.T) {
	randData := GenerateRandData(10 * 1024 * 1024)
	compressedData, _ := zstd.CompressLevel(nil, randData, 1)

	var streamBuffer bytes.Buffer
	w := zstd.NewWriterLevelDict(&streamBuffer, 1, nil)

	off := 0
	for {
		n := 512
		if len(randData[off:]) < n {
			n = len(randData[off:])
		}
		if n <= 0 {
			break
		}

		n, _ = w.Write(randData[off : off+n])
		off += n
		w.Flush()
	}
	w.Close()

	t.Logf("original length       : %d", len(randData))
	t.Logf("compress length       : %d", len(compressedData))
	t.Logf("stream compress length: %d", len(streamBuffer.Bytes()))

	decompressedData, _ := zstd.Decompress(nil, compressedData)

	if hex.EncodeToString(decompressedData) != hex.EncodeToString(randData) {
		t.Fatalf(EncodingTypeZstd + " buffer are different")
	}

	compressReader := zstd.NewReader(&streamBuffer)
	defer compressReader.Close()

	buffer2 := make([]byte, 512)
	var decompressedBuffer bytes.Buffer
	for {
		n, err := compressReader.Read(buffer2)
		decompressedBuffer.Write(buffer2[:n])
		if err != nil {
			break
		}
	}

	if hex.EncodeToString(decompressedBuffer.Bytes()) != hex.EncodeToString(randData) {
		t.Fatalf(EncodingTypeZstd + " buffer are different")
	}
}

func TestHttpEncodingReadCloser(t *testing.T) {

	dataLength := 1024 * 512

	for i := 0; i < 10; i++ {
		t.Run("with "+EncodingTypeNone, func(t2 *testing.T) {
			//일반 데이터를 읽는 테스트

			//일반 io.Writer 준비
			randData := GenerateRandData(dataLength)

			fname := hex.EncodeToString(GenerateRandData(16))
			writeFile(fname, randData)
			defer os.Remove(fname)

			//읽기
			file, _ := os.Open(fname)

			reader := NewEncodingReadCloser(file, EncodingTypeNone)
			defer reader.Close()

			buffer2 := make([]byte, rand.Intn(512)+32)
			var readBuffer bytes.Buffer

			for {
				n, err := reader.Read(buffer2)
				readBuffer.Write(buffer2[:n])
				if err != nil {
					break
				}
			}

			if hex.EncodeToString(randData) != hex.EncodeToString(readBuffer.Bytes()) {
				t.Fatalf(EncodingTypeNone + " buffer are different")
			}
		})
	}

	for i := 0; i < 10; i++ {
		t.Run("with "+EncodingTypeZstd, func(t2 *testing.T) {
			//스트림 방식으로 압축한 데이터를 읽는 테스트

			//스트림 방식 io.Writer 준비
			randData := GenerateRandData(dataLength)
			var randDataBuffer bytes.Buffer
			randDataBuffer.Write(randData)

			fname := hex.EncodeToString(GenerateRandData(16))
			file, _ := os.Create(fname)
			defer os.Remove(fname)

			w := zstd.NewWriterLevelDict(file, 1, nil)
			buffer1 := make([]byte, rand.Intn(512)+32)
			for {
				n, err := randDataBuffer.Read(buffer1)
				w.Write(buffer1[:n])
				if err != nil {
					break
				}
			}
			w.Close()
			file.Close()

			//읽기
			file, _ = os.Open(fname)
			reader := NewEncodingReadCloser(file, EncodingTypeZstd)
			defer reader.Close()

			buffer2 := make([]byte, rand.Intn(512)+32)
			var readBuffer bytes.Buffer

			for {
				n, err := reader.Read(buffer2)
				readBuffer.Write(buffer2[:n])
				if err != nil {
					break
				}
			}

			if hex.EncodeToString(randData) != hex.EncodeToString(readBuffer.Bytes()) {
				t.Fatalf(EncodingTypeZstd + " buffer are different")
			}
		})
	}
}

func TestBufferEncodingReader(t *testing.T) {

	dataLength := 1024 * 512
	randData := GenerateRandData(dataLength)

	for i := 0; i < 10; i++ {
		t.Run("with "+EncodingTypeNone, func(t2 *testing.T) {
			reader, _, _ := NewBufferEncodingReader(randData, EncodingTypeNone)

			buffer := make([]byte, rand.Intn(512)+32)
			var readBuffer bytes.Buffer

			for {
				n, err := reader.Read(buffer)
				readBuffer.Write(buffer[:n])
				if err != nil {
					break
				}
			}

			if hex.EncodeToString(randData) != hex.EncodeToString(readBuffer.Bytes()) {
				t.Fatalf(EncodingTypeZstd + " buffer are different")
			}
		})
	}

	for i := 0; i < 10; i++ {
		t.Run("with "+EncodingTypeZstd, func(t2 *testing.T) {
			compressedData, _ := zstd.CompressLevel(nil, randData, 1)

			reader, _, _ := NewBufferEncodingReader(randData, EncodingTypeZstd)

			buffer := make([]byte, rand.Intn(512)+32)
			var readBuffer bytes.Buffer

			for {
				n, err := reader.Read(buffer)
				readBuffer.Write(buffer[:n])
				if err != nil {
					break
				}
			}

			if hex.EncodeToString(compressedData) != hex.EncodeToString(readBuffer.Bytes()) {
				t.Fatalf(EncodingTypeZstd + " buffer are different")
			}
		})
	}
}

func TestFileEncodingWriteCloser(t *testing.T) {

	dataLength := 1024 * 512
	randData := GenerateRandData(dataLength)
	fname := hex.EncodeToString(GenerateRandData(16))

	writeFile(fname, randData)

	file, _ := os.Open(fname)
	defer func() {
		file.Close()
		os.Remove(fname)
	}()

	for i := 0; i < 10; i++ {
		t.Run("with "+EncodingTypeNone, func(t2 *testing.T) {
			var writeBuffer bytes.Buffer

			file.Seek(0, io.SeekStart)
			writer := NewEncodingWriteCloser(&writeBuffer, EncodingTypeNone)
			buffer := make([]byte, rand.Intn(512)+32)
			for {
				n, err := file.Read(buffer)
				writer.Write(buffer[:n])
				if err != nil {
					break
				}
			}
			writer.Close()

			if hex.EncodeToString(writeBuffer.Bytes()) != hex.EncodeToString(randData) {
				t.Fatalf(EncodingTypeZstd + " buffer are different")
			}
		})
	}

	for i := 0; i < 10; i++ {
		t.Run("with "+EncodingTypeZstd, func(t2 *testing.T) {
			var compressedBuffer bytes.Buffer

			file.Seek(0, io.SeekStart)
			writer := NewEncodingWriteCloser(&compressedBuffer, EncodingTypeZstd)
			buffer1 := make([]byte, rand.Intn(512)+32)
			for {
				n, err := file.Read(buffer1)
				writer.Write(buffer1[:n])
				if err != nil {
					break
				}
			}
			writer.Close()

			// 분할 스트림으로 압축한 데이터는 일반 방식으로 해제할 수 없음
			// 동일하게 분할 스트림으로 해제

			compressReader := zstd.NewReader(&compressedBuffer)
			defer compressReader.Close()

			buffer2 := make([]byte, rand.Intn(512)+32)
			var decompressedBuffer bytes.Buffer
			for {
				n, err := compressReader.Read(buffer2)
				decompressedBuffer.Write(buffer2[:n])
				if err != nil {
					break
				}
			}

			if hex.EncodeToString(decompressedBuffer.Bytes()) != hex.EncodeToString(randData) {
				t.Fatalf(EncodingTypeZstd + " buffer are different")
			}
		})
	}
}
