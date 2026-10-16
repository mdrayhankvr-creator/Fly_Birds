package academic

import (
	"context"
	"errors"
	"testing"
)

const testUUID = "11111111-1111-4111-8111-111111111111"
const secondUUID = "22222222-2222-4222-8222-222222222222"

func boolValue(v bool) *bool { return &v }

type serviceRepo struct {
	Repository
	class   Class
	subject Subject
	mapping Mapping
	query   Query
}

func (r *serviceRepo) SaveClass(_ context.Context, _, _ string, v Class) (Class, error) {
	r.class = v
	return v, nil
}
func (r *serviceRepo) SaveSubject(_ context.Context, _, _ string, v Subject) (Subject, error) {
	r.subject = v
	return v, nil
}
func (r *serviceRepo) CreateMapping(_ context.Context, _ string, v Mapping) (Mapping, error) {
	r.mapping = v
	return v, nil
}
func (r *serviceRepo) UpdateMapping(_ context.Context, _ string, v Mapping) (Mapping, error) {
	r.mapping = v
	return v, nil
}
func (r *serviceRepo) ListClasses(_ context.Context, q Query) (Page[Class], error) {
	r.query = q
	return Page[Class]{}, nil
}

func TestClassValidation(t *testing.T) {
	r := &serviceRepo{}
	s := NewService(r)
	ctx := context.Background()
	for _, input := range []ClassInput{{Name: "", IsActive: boolValue(true)}, {Name: "Class", DisplayOrder: -1, IsActive: boolValue(true)}, {Name: "Class", DisplayOrder: 100001, IsActive: boolValue(true)}, {Name: "Class"}, {Name: string(make([]byte, 121)), IsActive: boolValue(true)}} {
		if _, err := s.SaveClass(ctx, testUUID, "", input); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected invalid class, got %v", err)
		}
	}
	value, err := s.SaveClass(ctx, testUUID, "", ClassInput{Name: "  Class Nine  ", DisplayOrder: 9, IsActive: boolValue(false)})
	if err != nil || value.Name != "Class Nine" || value.DisplayOrder != 9 || value.IsActive {
		t.Fatalf("unexpected class: %+v %v", value, err)
	}
	if _, err := s.SaveClass(ctx, "bad", "", ClassInput{Name: "Class", IsActive: boolValue(true)}); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid actor accepted")
	}
}
func TestSubjectValidation(t *testing.T) {
	r := &serviceRepo{}
	s := NewService(r)
	ctx := context.Background()
	for _, input := range []SubjectInput{{NameEN: "Math", Code: "MAT", IsActive: boolValue(true)}, {NameEN: "Math", NameBN: "গণিত", Code: "bad code", IsActive: boolValue(true)}, {NameEN: "Math", NameBN: "গণিত", Code: "", IsActive: boolValue(true)}, {NameEN: "Math", NameBN: "গণিত", Code: "MAT"}} {
		if _, err := s.SaveSubject(ctx, testUUID, "", input); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected invalid subject, got %v", err)
		}
	}
	value, err := s.SaveSubject(ctx, testUUID, "", SubjectInput{NameEN: " Mathematics ", NameBN: " গণিত ", Code: " mat-101 ", IsActive: boolValue(true)})
	if err != nil || value.NameEN != "Mathematics" || value.NameBN != "গণিত" || value.Code != "MAT-101" {
		t.Fatalf("unexpected subject: %+v %v", value, err)
	}
}
func TestMappingSemantics(t *testing.T) {
	r := &serviceRepo{}
	s := NewService(r)
	ctx := context.Background()
	for _, flags := range [][2]*bool{{nil, nil}, {boolValue(true), nil}, {boolValue(true), boolValue(true)}, {boolValue(false), boolValue(false)}} {
		_, err := s.CreateMapping(ctx, testUUID, secondUUID, MappingInput{SubjectID: testUUID, IsRequired: flags[0], IsFourthSubject: flags[1]})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected invalid mapping flags, got %v", err)
		}
	}
	for _, fourth := range []bool{false, true} {
		value, err := s.CreateMapping(ctx, testUUID, secondUUID, MappingInput{SubjectID: testUUID, IsRequired: boolValue(!fourth), IsFourthSubject: boolValue(fourth)})
		if err != nil || value.IsRequired == value.IsFourthSubject {
			t.Fatalf("unexpected mapping: %+v %v", value, err)
		}
		value, err = s.UpdateMapping(ctx, testUUID, secondUUID, testUUID, MappingInput{IsRequired: boolValue(!fourth), IsFourthSubject: boolValue(fourth)})
		if err != nil || value.IsFourthSubject != fourth {
			t.Fatalf("unexpected updated mapping: %+v %v", value, err)
		}
	}
	if _, err := s.UpdateMapping(ctx, testUUID, secondUUID, testUUID, MappingInput{SubjectID: secondUUID, IsRequired: boolValue(true), IsFourthSubject: boolValue(false)}); !errors.Is(err, ErrInvalid) {
		t.Fatal("mapping subject reassignment accepted")
	}
}
func TestAcademicPaginationValidation(t *testing.T) {
	r := &serviceRepo{}
	s := NewService(r)
	ctx := context.Background()
	for _, q := range []Query{{Page: 0, PageSize: 10}, {Page: 1, PageSize: 0}, {Page: 1, PageSize: 101}, {Page: 100001, PageSize: 1}, {Page: 1, PageSize: 1, Status: "other"}} {
		if _, err := s.ListClasses(ctx, q); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid query accepted: %+v", q)
		}
	}
	if _, err := s.ListClasses(ctx, Query{Search: "  Nine  ", Page: 2, PageSize: 10, Status: "active"}); err != nil || r.query.Search != "Nine" {
		t.Fatalf("query normalization failed: %+v %v", r.query, err)
	}
}
