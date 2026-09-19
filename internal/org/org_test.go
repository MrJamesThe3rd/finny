package org_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/MrJamesThe3rd/finny/internal/org"
)

func TestValidateNIF(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		give    string
		wantErr bool
	}{
		{
			name: "AbsentIsValid",
			give: "",
		},
		{
			name: "VibrantGarden",
			give: "517948974",
		},
		{
			name: "CheckDigitRollsOverToZero",
			give: "999999990",
		},
		{
			name:    "WrongCheckDigit",
			give:    "517948975",
			wantErr: true,
		},
		{
			name:    "TooShort",
			give:    "51794897",
			wantErr: true,
		},
		{
			name:    "TooLong",
			give:    "5179489744",
			wantErr: true,
		},
		{
			name:    "NonDigit",
			give:    "51794897a",
			wantErr: true,
		},
		{
			name:    "NonDigitInBody",
			give:    "5179a8974",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := org.ValidateNIF(tt.give)

			if tt.wantErr {
				assert.ErrorIs(t, err, org.ErrInvalidNIF)
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestRole_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give org.Role
		want bool
	}{
		{name: "Owner", give: org.RoleOwner, want: true},
		{name: "Accountant", give: org.RoleAccountant, want: true},
		{name: "Empty", give: org.Role(""), want: false},
		{name: "Unknown", give: org.Role("admin"), want: false},
		{name: "WrongCase", give: org.Role("Owner"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.give.Valid())
		})
	}
}
