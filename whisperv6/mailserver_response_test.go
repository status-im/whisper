package whisperv6

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/syndtr/goleveldb/leveldb/errors"
)

func checkValidErrorPayload(t *testing.T, id []byte, errorMsg string) {
	requestID := common.BytesToHash(id)

	errPayload := CreateMailServerRequestFailedPayload(requestID, errors.New(errorMsg))

	event, err := CreateMailServerEvent(errPayload)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
		return
	}

	if event == nil {
		t.Errorf("Could not parse payload: %v", errPayload)
		return
	}

	if !bytes.Equal(event.Hash[:], requestID[:]) {
		t.Errorf("Unexpected hash: %v, expected %v", event.Hash, requestID)
		return
	}

	eventData, ok := event.Data.(*MailServerResponse)
	if !ok {
		t.Errorf("Unexpected data in event: %v, expected a MailServerResponse", event.Data)
		return
	}

	if strings.Compare(eventData.Error.Error(), errorMsg) != 0 {
		t.Errorf("Unexpected error string: '%s', expected '%s'", eventData, errorMsg)
		return
	}
}

func checkValidSuccessPayload(t *testing.T, id []byte, lastHash []byte, timestamp uint32, envHash []byte) {
	requestID := common.BytesToHash(id)
	lastEnvelopeHash := common.BytesToHash(lastHash)

	timestampBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(timestampBytes, timestamp)

	envelopeHash := common.BytesToHash(envHash)

	cursor := append(timestampBytes, envelopeHash[:]...)

	successPayload := CreateMailServerRequestCompletedPayload(requestID, lastEnvelopeHash, cursor)

	event, err := CreateMailServerEvent(successPayload)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
		return
	}

	if event == nil {
		t.Errorf("Could not parse payload: %v", successPayload)
		return
	}

	if !bytes.Equal(event.Hash[:], requestID[:]) {
		t.Errorf("Unexpected hash: %v, expected %v", event.Hash, requestID)
		return
	}

	eventData, ok := event.Data.(*MailServerResponse)
	if !ok {
		t.Errorf("Unexpected data in event: %v, expected a MailServerResponse", event.Data)
		return
	}

	if !bytes.Equal(eventData.LastEnvelopeHash[:], lastEnvelopeHash[:]) {
		t.Errorf("Unexpected LastEnvelopeHash: %v, expected %v",
			eventData.LastEnvelopeHash, lastEnvelopeHash)
		return
	}

	if !bytes.Equal(eventData.Cursor, cursor) {
		t.Errorf("Unexpected cursor: %v, expected: %v", eventData.Cursor, cursor)
		return
	}

	if eventData.Error != nil {
		t.Errorf("Unexpected error: %v", eventData.Error)
		return
	}
}

func TestCreateMailServerEvent(t *testing.T) {
	// valid cases
	longErrorMessage := "longMessage|"
	for i := 0; i < 5; i++ {
		longErrorMessage = longErrorMessage + longErrorMessage
	}
	checkValidErrorPayload(t, []byte{0x01}, "test error 1")
	checkValidErrorPayload(t, []byte{0x02}, "test error 2")
	checkValidErrorPayload(t, []byte{0x02}, "")
	checkValidErrorPayload(t, []byte{0x00}, "test error 3")
	checkValidErrorPayload(t, []byte{}, "test error 4")

	checkValidSuccessPayload(t, []byte{0x01}, []byte{0x02}, 123, []byte{0x03})
	// invalid payloads

	// too small
	_, err := CreateMailServerEvent([]byte{0x00})
	if err == nil {
		t.Errorf("Expected an error, got nil")
		return
	}

	// too big and not error payload
	payloadTooBig := make([]byte, common.HashLength*2+cursorSize+100)
	_, err = CreateMailServerEvent(payloadTooBig)
	if err == nil {
		t.Errorf("Expected an error, got nil")
		return
	}

}
