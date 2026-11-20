package domain

import (
	"strings"
	"time"
)

// TONDomain is an owned .ton name in the DeltaGram registry.
type TONDomain struct {
	Name        string    `json:"name"`
	OwnerID     int64     `json:"owner_id"`
	OwnerName   string    `json:"owner_name,omitempty"`
	OwnerUser   string    `json:"owner_username,omitempty"`
	AcquiredAt  time.Time `json:"acquired_at"`
	LastPrice   int64     `json:"last_price_nanoton"`
	Listed      bool      `json:"listed"`
	Ask         int64     `json:"ask_nanoton"`
}

// TONDomainState is what the search field shows after a name is checked.
type TONDomainState string

const (
	TONDomainAvailable TONDomainState = "available"
	TONDomainTaken     TONDomainState = "taken"
	TONDomainYours     TONDomainState = "yours"
	TONDomainForSale   TONDomainState = "for_sale"
)

// TONDomainCheck is the result of a name lookup.
type TONDomainCheck struct {
	Name      string         `json:"name"`
	State     TONDomainState `json:"state"`
	Price     int64          `json:"price_nanoton"`
	BasePrice int64          `json:"base_price_nanoton"`
	OwnerID   int64          `json:"owner_id,omitempty"`
	OwnerName string         `json:"owner_name,omitempty"`
	OwnerUser string         `json:"owner_username,omitempty"`
}

// NormalizeTONDomain lowercases a name, strips "@" and a trailing ".ton" and
// validates the alphabet accepted by TON DNS. It returns "" for invalid names.
func NormalizeTONDomain(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.TrimPrefix(name, "@")
	name = strings.TrimSuffix(name, ".ton")
	if name == "" || len(name) > 32 {
		return ""
	}
	for _, c := range name {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return ""
		}
	}
	return name
}

// TONDomainBasePrice returns the registry price for an unowned name. Short
// names are scarce, so they cost more; the ladder mirrors the TON DNS feel
// without depending on an on-chain auction.
func TONDomainBasePrice(name string) int64 {
	var ton int64
	switch n := len(name); {
	case n <= 1:
		ton = 500
	case n == 2:
		ton = 250
	case n == 3:
		ton = 100
	case n == 4:
		ton = 50
	case n == 5:
		ton = 25
	case n == 6:
		ton = 15
	case n == 7:
		ton = 10
	case n <= 10:
		ton = 7
	default:
		ton = 5
	}
	return ton * NanotonPerTON
}
