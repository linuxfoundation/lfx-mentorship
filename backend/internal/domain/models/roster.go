// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// RosterMember is an LFID-backed membership in an authorization roster.
type RosterMember struct {
	ObjectUID string    `json:"object_uid"`
	UserID    string    `json:"user_id"`
	LFID      string    `json:"lfid"`
	CreatedOn time.Time `json:"created_on"`
	UpdatedOn time.Time `json:"updated_on"`
}
