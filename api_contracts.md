# API Контракты Канбана (Ожидания React Фронтенда)

Этот файл описывает точную структуру JSON, которую ожидает React-фронтенд (на основе TypeScript-интерфейсов из `analytics_platform/src/types/features/projects.ts`).
Микросервис на Go должен отдавать ответы в строгом соответствии с этой структурой.

## 1. Детали проекта (GET /spa/api/projects/{id})

```json
{
  "id": 26,
  "name": "Название проекта",
  "description": "Описание (может быть null)",
  "createdAt": "2026-06-30T10:00:00Z",
  "updatedAt": "2026-06-30T10:00:00Z",
  
  "owner": {
    "id": 144,
    "login": "ivanov_ii",
    "lastname": "Иванов",
    "firstname": "Иван",
    "patronymic": "Иванович"
  },
  
  "isOwner": true,
  "isProjectAdmin": true,
  "memberRole": "KANBAN_ADMIN",
  
  "boards": [
    {
      "id": 1,
      "title": "Главная доска",
      "position": 1.0,
      "updatedAt": "2026-06-30T10:05:00Z"
    }
  ],
  
  "members": [
    {
      "userId": 144,
      "login": "ivanov_ii",
      "lastname": "Иванов",
      "firstname": "Иван",
      "patronymic": "Иванович",
      "profession": "Разработчик",
      "avatarUrl": "https://...",
      "role": "KANBAN_ADMIN",
      "roleLabel": "Администратор",
      "isOwner": true
    }
  ]
}
```

## 2. Данные доски (GET /spa/api/projects/{id}/boards/{boardId})

Здесь фронтенд ждет полную структуру колонок и вложенных карточек.

```json
{
  "id": 1,
  "title": "Главная доска",
  "updatedAt": "2026-06-30T10:05:00Z",
  "columns": [
    {
      "id": 1,
      "title": "В работе",
      "headerColor": "bg-primary",
      "position": 1.0,
      "cards": [
        {
          "id": 10,
          "title": "Название карточки",
          "description": "Описание",
          "position": 1.0,
          "priority": "high",
          "dueDate": "2026-07-10T12:00:00Z",
          "borderColor": null,
          "updatedAt": "2026-06-30T10:05:00Z",
          "checklistTotal": 5,
          "checklistDone": 2,
          "commentsCount": 3,
          "labels": [
            {
              "id": 1,
              "name": "Bug",
              "color": "red"
            }
          ],
          "assignees": [
            {
              "id": 144,
              "name": "Иванов И.И.",
              "avatarUrl": "https://..."
            }
          ]
        }
      ]
    }
  ]
}
```

## 3. Детали карточки для сайдбара (GET /spa/api/cards/{id})

```json
{
  "id": 10,
  "title": "Название карточки",
  "description": "Описание",
  "position": 1.0,
  "priority": "high",
  "priorityLabel": "Высокий",
  "priorityColor": "danger",
  "dueDate": "2026-07-10T12:00:00Z",
  "columnId": 1,
  "columnTitle": "В работе",
  "boardId": 1,
  "isArchived": false,
  "borderColor": null,
  "createdAt": "2026-06-30T10:00:00Z",
  "updatedAt": "2026-06-30T10:05:00Z",
  
  "labels": [...],
  "assignees": [...],
  
  "createdBy": {
    "id": 144,
    "firstname": "Иван",
    "lastname": "Иванов"
  },
  
  "comments": [
    {
      "id": 1,
      "body": "Текст комментария",
      "authorName": "Иванов И.И.",
      "authorId": 144,
      "createdAt": "2026-06-30T10:05:00Z",
      "updatedAt": "2026-06-30T10:05:00Z"
    }
  ],
  
  "attachments": [
    {
      "id": 1,
      "filename": "документ.pdf",
      "contentType": "application/pdf",
      "sizeBytes": 1024,
      "context": "card",
      "previewUrl": "https://...",
      "authorId": 144,
      "authorName": "Иванов И.И.",
      "createdAt": "2026-06-30T10:05:00Z"
    }
  ],
  
  "subtasks": [
    {
      "id": 1,
      "title": "Подзадача 1",
      "status": "done",
      "isCompleted": true,
      "position": 1.0,
      "userId": 144,
      "userName": "Иванов И.И."
    }
  ]
}
```

## 4. История активности карточки (GET /spa/api/cards/{id}/activities)

```json
{
  "items": [
    {
      "type": "status_change",
      "label": "изменил статус",
      "icon": "status",
      "oldValue": "В работе",
      "newValue": "Проверены",
      "createdAt": "2026-06-30T10:05:00Z",
      "user": {
        "id": 144,
        "name": "Иванов И.И."
      }
    }
  ],
  "hasMore": false,
  "nextOffset": 0
}
```
