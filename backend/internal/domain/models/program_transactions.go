// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "time"

// ProgramTransaction mirrors the crowdfunding categorized transaction wire format.
type ProgramTransaction struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	AmountCents    int64     `json:"amount_cents"`
	Date           time.Time `json:"date"`
	Category       string    `json:"category,omitempty"`
	Recurring      bool      `json:"recurring"`
	InitiativeName string    `json:"initiative_name,omitempty"`
	DonorName      string    `json:"donor_name,omitempty"`
	DonorType      string    `json:"donor_type,omitempty"`
	DonorLogoURL   string    `json:"donor_logo_url,omitempty"`
	DonorUsername  string    `json:"donor_username,omitempty"`
}

// ProgramCategorizedTransactions groups transactions by donor type.
type ProgramCategorizedTransactions struct {
	IndividualTransactions   []ProgramTransaction `json:"individual_transactions"`
	OrganizationTransactions []ProgramTransaction `json:"organization_transactions"`
	TotalCount               int                  `json:"total_count"`
	Limit                    int                  `json:"limit"`
	Offset                   int                  `json:"offset"`
}

// ProgramSponsor is an aggregated sponsor card payload for program detail views.
type ProgramSponsor struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	LogoURL     string `json:"logo_url,omitempty"`
	AmountCents int64  `json:"amount_cents"`
}
