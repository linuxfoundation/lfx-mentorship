// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package models defines the domain model types shared across the application.
package models

import "time"

// User maps to the public.users table.
type User struct {
	ID         string    `json:"id"`
	Email      *string   `json:"email,omitempty"`
	LFID       *string   `json:"lfid,omitempty"`
	Name       *string   `json:"name,omitempty"`
	GivenName  *string   `json:"given_name,omitempty"`
	FamilyName *string   `json:"family_name,omitempty"`
	AvatarURL  *string   `json:"avatar_url,omitempty"`
	CreatedOn  time.Time `json:"created_on"`
	UpdatedOn  time.Time `json:"updated_on"`
}

// UserCreateInput is the request body for creating a user.
type UserCreateInput struct {
	ID         string  `json:"id"`
	Email      *string `json:"email,omitempty"`
	LFID       *string `json:"lfid,omitempty"`
	Name       *string `json:"name,omitempty"`
	GivenName  *string `json:"given_name,omitempty"`
	FamilyName *string `json:"family_name,omitempty"`
	AvatarURL  *string `json:"avatar_url,omitempty"`
}

// UserUpdateInput is the request body for updating a user.
type UserUpdateInput struct {
	Email      *string `json:"email,omitempty"`
	LFID       *string `json:"lfid,omitempty"`
	Name       *string `json:"name,omitempty"`
	GivenName  *string `json:"given_name,omitempty"`
	FamilyName *string `json:"family_name,omitempty"`
	AvatarURL  *string `json:"avatar_url,omitempty"`
}

// Principal holds the authenticated caller's identity extracted from the JWT.
type Principal struct {
	UserID        string
	Username      string
	Email         string
	EmailVerified bool
	Scope         string
	Name          string
	GivenName     string
	FamilyName    string
	Picture       string
}
