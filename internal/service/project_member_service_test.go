package service

import (
	"context"
	"testing"

	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type folderProjectStub struct {
	repository.ProjectRepositoryInterface
}

func (folderProjectStub) GetProject(context.Context, int64) (*model.Project, error) {
	return &model.Project{ID: 1, OwnerID: 1}, nil
}

type folderUsersStub struct {
	repository.UserRepositoryInterface
}

func (folderUsersStub) GetUsersByIDs(_ context.Context, ids []int64) ([]model.User, error) {
	users := make([]model.User, len(ids))
	for i, id := range ids {
		users[i].ID = id
	}
	return users, nil
}

type folderMembersStub struct {
	repository.ProjectMemberRepositoryInterface
	got []model.ProjectUser
}

func (*folderMembersStub) GetMembers(context.Context, int64) ([]model.ProjectUser, error) {
	return nil, nil
}
func (s *folderMembersStub) ReplaceMembers(_ context.Context, _ int64, m []model.ProjectUser) error {
	s.got = m
	return nil
}

func TestReplaceMembersDoesNotAssignPersonalFolders(t *testing.T) {
	members := &folderMembersStub{}
	perm := NewPermissionService(nil, folderProjectStub{}, members)
	svc := NewProjectMemberService(members, folderUsersStub{}, perm, nil)
	ctx := context.WithValue(context.Background(), middleware.UserContextKey, middleware.UserClaims{ID: 1})
	folder := int64(99)
	if err := svc.ReplaceMembers(ctx, 1, []dto.AddProjectMemberRequest{{UserID: 2, Role: "KANBAN_EDITOR", FolderID: &folder}}); err != nil {
		t.Fatal(err)
	}
	if len(members.got) != 2 {
		t.Fatalf("members=%v", members.got)
	}
	for _, m := range members.got {
		if m.FolderID != nil {
			t.Fatalf("assigned personal folder to %d", m.UserID)
		}
	}
}

// Пустой записи в истории быть не должно: при неизменном составе
// ReplaceMembers выходит раньше записи, при любом отличии — пишет.
func TestSameMembers(t *testing.T) {
	folder := int64(3)
	before := []model.ProjectUser{
		{UserID: 1, Role: "KANBAN_ADMIN"},
		{UserID: 2, Role: "KANBAN_EDITOR"},
	}
	tests := []struct {
		name  string
		after []model.ProjectUser
		want  bool
	}{
		{"тот же состав в другом порядке, папка не в счёт", []model.ProjectUser{
			{UserID: 2, Role: "KANBAN_EDITOR", FolderID: &folder}, {UserID: 1, Role: "KANBAN_ADMIN"},
		}, true},
		{"сменили роль", []model.ProjectUser{
			{UserID: 1, Role: "KANBAN_ADMIN"}, {UserID: 2, Role: "KANBAN_VIEWER"},
		}, false},
		{"добавили", []model.ProjectUser{
			{UserID: 1, Role: "KANBAN_ADMIN"}, {UserID: 2, Role: "KANBAN_EDITOR"}, {UserID: 3, Role: "KANBAN_EDITOR"},
		}, false},
		{"убрали", []model.ProjectUser{{UserID: 1, Role: "KANBAN_ADMIN"}}, false},
		{"заменили одного другим", []model.ProjectUser{
			{UserID: 1, Role: "KANBAN_ADMIN"}, {UserID: 3, Role: "KANBAN_EDITOR"},
		}, false},
	}
	for _, tt := range tests {
		if got := sameMembers(before, tt.after); got != tt.want {
			t.Errorf("%s: sameMembers = %v, want %v", tt.name, got, tt.want)
		}
	}
}
