// Copyright 2020 Condensat Tech. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package services

import (
	"errors"
	"net/http"

	apiservice "github.com/condensat/bank-core/api/services"
	"github.com/condensat/bank-core/api/sessions"
	"github.com/condensat/bank-core/appcontext"
	"github.com/condensat/bank-core/database"
	"github.com/condensat/bank-core/database/model"
	"github.com/sirupsen/logrus"

	"github.com/condensat/bank-core/logger"
)

var (
	ErrPermissionDenied = errors.New("Permission Denied")
)

type DashboardService int

// StatusRequest holds args for status requests
type StatusRequest struct {
	apiservice.SessionArgs
}

type UsersStatus struct {
	Count     int `json:"count"`
	Connected int `json:"connected"`
}

type CurrencyBalance struct {
	Currency string  `json:"currency"`
	Balance  float64 `json:"balance"`
	Locked   float64 `json:"locked,omitempty"`
}

type AccountingStatus struct {
	Count    int               `json:"count"`
	Active   int               `json:"active"`
	Balances []CurrencyBalance `json:"balances"`
}

// StatusResponse holds args for string requests
type StatusResponse struct {
	Users      UsersStatus      `json:"users"`
	Accounting AccountingStatus `json:"accounting"`
}

func (p *DashboardService) Status(r *http.Request, request *StatusRequest, reply *StatusResponse) error {
	ctx := r.Context()
	log := logger.Logger(ctx).WithField("Method", "services.DashboardService.Status")
	log = apiservice.GetServiceRequestLog(log, r, "Dashboard", "Status")

	db := appcontext.Database(ctx)
	session, err := sessions.ContextSession(ctx)
	if err != nil {
		return apiservice.ErrServiceInternalError
	}

	// Get userID from session
	request.SessionID = apiservice.GetSessionCookie(r)
	sessionID := sessions.SessionID(request.SessionID)
	userID := session.UserSession(ctx, sessionID)
	if !sessions.IsUserValid(userID) {
		log.Error("Invalid userSession")
		return sessions.ErrInvalidSessionID
	}
	log = log.WithFields(logrus.Fields{
		"SessionID": sessionID,
		"UserID":    userID,
	})

	isAdmin, err := database.UserHasRole(db, model.UserID(userID), model.RoleNameAdmin)
	if err != nil {
		log.WithError(err).
			WithField("RoleName", model.RoleNameAdmin).
			Error("UserHasRole failed")
		return ErrPermissionDenied
	}

	if !isAdmin {
		log.WithError(err).
			Error("User is not Admin")
		return ErrPermissionDenied
	}

	userCount, err := database.UserCount(db)
	if err != nil {
		return apiservice.ErrServiceInternalError
	}
	sessionCount, err := session.Count(ctx)
	if err != nil {
		return apiservice.ErrServiceInternalError
	}

	accountsInfo, err := database.AccountsInfos(db)
	if err != nil {
		log.WithError(err).
			Error("AccountInfos failed")
		return apiservice.ErrServiceInternalError
	}

	var balances []CurrencyBalance
	for _, account := range accountsInfo.Accounts {
		balances = append(balances, CurrencyBalance{
			Currency: account.CurrencyName,
			Balance:  account.Balance,
			Locked:   account.TotalLocked,
		})
	}

	*reply = StatusResponse{
		Users: UsersStatus{
			Count:     userCount,
			Connected: sessionCount,
		},
		Accounting: AccountingStatus{
			Count:    accountsInfo.Count,
			Active:   accountsInfo.Active,
			Balances: balances,
		},
	}

	log.WithFields(logrus.Fields{
		"UserCount":    userCount,
		"SessionCount": sessionCount,
	}).Info("Status")

	return nil
}
