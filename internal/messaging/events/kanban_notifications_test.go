package events

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// Имена полей — это и есть контракт с сервисом нотификаций: он разбирает
// тело по json-тегам. Опечатка в теге не сломает сборку ни здесь, ни там —
// уведомление просто приедет с пустым заголовком. Молча, как всё в этой
// подсистеме до переделки. Отсюда golden-проверка.
//
// Парная проверка на стороне консьюмера:
// go_notification_service_document_flow/internal/service/contract_test.go
const goldenPayload = `{` +
	`"eventId":"018f0c3e-7a11-7c9d-9f2a-4d5b6e7f8a90",` +
	`"title":"Новая задача «Свет в холле» на доске «Спринт 12»",` +
	`"typeLabel":"Создана задача",` +
	// `&` в ссылке уезжает как \u0026: encoding/json по умолчанию экранирует
	// HTML-символы. На проводе это валидный JSON, консьюмер разбирает обратно
	// в `&` — парная проверка на его стороне это и подтверждает.
	`"link":"/projects/7?board=3\u0026task=42",` +
	`"type":"card_created",` +
	`"actorId":144,` +
	`"projectId":7,` +
	`"boardId":3,` +
	`"cardId":42,` +
	`"recipients":[187,1745],` +
	`"data":{"boardTitle":"Спринт 12"}` +
	`}`

func TestKanbanNotificationEventJSON(t *testing.T) {
	boardID, cardID := int64(3), int64(42)
	link := "/projects/7?board=3&task=42"

	evt := KanbanNotificationEvent{
		EventID:    "018f0c3e-7a11-7c9d-9f2a-4d5b6e7f8a90",
		Title:      "Новая задача «Свет в холле» на доске «Спринт 12»",
		TypeLabel:  "Создана задача",
		Link:       &link,
		Type:       "card_created",
		ActorID:    144,
		ProjectID:  7,
		BoardID:    &boardID,
		CardID:     &cardID,
		Recipients: []int64{187, 1745},
		Data:       map[string]any{"boardTitle": "Спринт 12"},
	}

	got, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if string(got) != goldenPayload {
		t.Errorf("формат события разошёлся с контрактом\nполучили: %s\nожидали:  %s", got, goldenPayload)
	}
}

// Message не заполняется канбаном, и в теле его быть не должно: у поля стоит
// omitempty, иначе консьюмер получал бы null там, где ожидает отсутствие.
func TestKanbanNotificationEventOmitsEmptyOptionals(t *testing.T) {
	evt := KanbanNotificationEvent{
		EventID:    NewEventID(),
		Title:      "Вас добавили в проект «Ремонт»",
		Recipients: []int64{187},
	}

	got, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, absent := range []string{`"message"`, `"link"`, `"typeLabel"`, `"boardId"`, `"cardId"`} {
		if contains(string(got), absent) {
			t.Errorf("пустое поле %s не должно попадать в тело: %s", absent, got)
		}
	}
}

func TestNewEventIDIsUUIDv7(t *testing.T) {
	id, err := uuid.Parse(NewEventID())
	if err != nil {
		t.Fatalf("невалидный uuid: %v", err)
	}
	if got := id.Version(); got != 7 {
		t.Errorf("получили uuid v%d, ожидали v7: у v4 вставки ложатся в индекс вразброс", got)
	}

	// Один на событие — два вызова обязаны различаться.
	if NewEventID() == NewEventID() {
		t.Error("два вызова вернули одинаковый id")
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
