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
	"github.com/condensat/bank-core/utils"

	wallet "github.com/condensat/bank-core/wallet/client"

	"github.com/condensat/bank-core/logger"
	logmodel "github.com/condensat/bank-core/logger/model"

	"github.com/jinzhu/gorm"
	"github.com/sirupsen/logrus"
)

var (
	ErrPermissionDenied = errors.New("Permission Denied")
)

type DashboardService int

// StatusRequest holds args for status requests
type StatusRequest struct {
	apiservice.SessionArgs
}

type LogStatus struct {
	Warnings int `json:"warning"`
	Errors   int `json:"errors"`
	Panics   int `json:"panics"`
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

type BatchStatus struct {
	Count      int `json:"count"`
	Processing int `json:"processing"`
}

type WithdrawStatus struct {
	Count      int `json:"count"`
	Processing int `json:"processing"`
}

type WalletInfo struct {
	UTXOs  int     `json:"utxos"`
	Amount float64 `json:"amount"`
}

type WalletStatus struct {
	Chain  string     `json:"chain"`
	Total  WalletInfo `json:"total"`
	Locked WalletInfo `json:"locked"`
}

type ReserveStatus struct {
	Wallets []WalletStatus `json:"wallets"`
}

// StatusResponse holds args for string requests
type StatusResponse struct {
	Logs       LogStatus        `json:"logs"`
	Users      UsersStatus      `json:"users"`
	Accounting AccountingStatus `json:"accounting"`
	Batch      BatchStatus      `json:"batch"`
	Withdraw   WithdrawStatus   `json:"withdraw"`
	Reserve    ReserveStatus    `json:"reserve"`
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

	logsInfo, err := logmodel.LogsInfo(db.DB().(*gorm.DB))
	if err != nil {
		log.WithError(err).
			Error("LogsInfo failed")
		return apiservice.ErrServiceInternalError
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

	batchs, err := database.BatchsInfos(db)
	if err != nil {
		log.WithError(err).
			Error("BatchsInfos failed")
		return apiservice.ErrServiceInternalError
	}

	witdthdraws, err := database.WithdrawsInfos(db)
	if err != nil {
		log.WithError(err).
			Error("WithdrawsInfos failed")
		return apiservice.ErrServiceInternalError
	}

	walletStatus, err := wallet.WalletStatus(ctx)
	if err != nil {
		log.WithError(err).
			Error("WalletStatus failed")
		return apiservice.ErrServiceInternalError
	}

	var wallets []WalletStatus
	for _, wallet := range walletStatus.Wallets {
		var total float64
		var locked float64
		var lockedCount int
		for _, utxo := range wallet.UTXOs {
			total += utxo.Amount
			if utxo.Locked {
				locked += utxo.Amount
				lockedCount++
			}
		}
		wallets = append(wallets, WalletStatus{
			Chain: wallet.Chain,
			Total: WalletInfo{
				UTXOs:  len(wallet.UTXOs),
				Amount: utils.ToFixed(total, 8),
			},
			Locked: WalletInfo{
				UTXOs:  lockedCount,
				Amount: utils.ToFixed(locked, 8),
			},
		})
	}

	*reply = StatusResponse{
		Logs: LogStatus{
			Warnings: logsInfo.Warnings,
			Errors:   logsInfo.Errors,
			Panics:   logsInfo.Panics,
		},
		Users: UsersStatus{
			Count:     userCount,
			Connected: sessionCount,
		},
		Accounting: AccountingStatus{
			Count:    accountsInfo.Count,
			Active:   accountsInfo.Active,
			Balances: balances,
		},
		Batch: BatchStatus{
			Count:      batchs.Count,
			Processing: batchs.Active,
		},
		Withdraw: WithdrawStatus{
			Count:      witdthdraws.Count,
			Processing: witdthdraws.Active,
		},
		Reserve: ReserveStatus{
			Wallets: wallets,
		},
	}

	log.WithFields(logrus.Fields{
		"UserCount":    userCount,
		"SessionCount": sessionCount,
	}).Info("Status")

	return nil
}
