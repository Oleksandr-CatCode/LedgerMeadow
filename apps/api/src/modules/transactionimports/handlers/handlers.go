package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/transactionimports/models"
	importservices "ledgermeadow/src/modules/transactionimports/services"
	"ledgermeadow/src/modules/transactionimports/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
)

const maxMultipartBytes int64 = validators.MaxFileBytes + (64 << 10)

type Service interface {
	Preview(context.Context, shared.UserID, io.Reader) (models.Preview, error)
	Import(context.Context, shared.UserID, *int64, io.Reader) (models.ImportResult, error)
}

type Handler struct{ service Service }

func New(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) Preview(response http.ResponseWriter, request *http.Request) {
	form, err := readMultipart(response, request, false)
	if err != nil {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_RBC_CSV", "Select an unmodified RBC CSV file up to 2 MiB.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	preview, err := h.service.Preview(request.Context(), userID, bytes.NewReader(form.file))
	if errors.Is(err, importservices.ErrInvalidImport) {
		httpx.WriteError(response, http.StatusUnprocessableEntity, "INVALID_RBC_CSV", "The file is not a supported single-account RBC transaction CSV.")
		return
	}
	if errors.Is(err, importservices.ErrAccountConflict) {
		httpx.WriteError(response, http.StatusConflict, "RBC_CSV_ACCOUNT_CONFLICT", "More than one imported account matches this file.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "The RBC CSV could not be previewed.")
		return
	}
	httpx.WriteJSON(response, http.StatusOK, previewResponse(preview))
}

func (h *Handler) Import(response http.ResponseWriter, request *http.Request) {
	form, err := readMultipart(response, request, true)
	if err != nil {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_RBC_CSV_IMPORT", "The RBC CSV import request is invalid.")
		return
	}
	balance, err := optionalBalance(form.currentBalanceMinor)
	if err != nil {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_RBC_CSV_IMPORT", "Enter a non-negative current balance.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	result, err := h.service.Import(request.Context(), userID, balance, bytes.NewReader(form.file))
	if errors.Is(err, importservices.ErrInvalidImport) {
		httpx.WriteError(response, http.StatusUnprocessableEntity, "INVALID_RBC_CSV_IMPORT", "The account details or RBC CSV file are invalid.")
		return
	}
	if errors.Is(err, importservices.ErrAccountConflict) {
		httpx.WriteError(response, http.StatusConflict, "RBC_CSV_ACCOUNT_CONFLICT", "More than one imported account matches this file.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "The RBC CSV transactions could not be imported.")
		return
	}
	httpx.WriteJSON(response, http.StatusCreated, map[string]any{
		"account_id":             string(result.AccountID),
		"connection_id":          string(result.ConnectionID),
		"imported_count":         result.ImportedCount,
		"duplicate_count":        result.DuplicateCount,
		"materialization_status": "QUEUED",
	})
}

type multipartForm struct {
	file                []byte
	currentBalanceMinor string
}

func readMultipart(response http.ResponseWriter, request *http.Request, includeAccountFields bool) (multipartForm, error) {
	request.Body = http.MaxBytesReader(response, request.Body, maxMultipartBytes)
	reader, err := request.MultipartReader()
	if err != nil {
		return multipartForm{}, err
	}
	var form multipartForm
	seen := make(map[string]bool, 2)
	parts := 0
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return multipartForm{}, err
		}
		parts++
		if parts > 2 {
			part.Close()
			return multipartForm{}, errors.New("too many multipart fields")
		}
		name := part.FormName()
		if name == "" || seen[name] {
			part.Close()
			return multipartForm{}, errors.New("invalid multipart field")
		}
		seen[name] = true
		switch name {
		case "file":
			form.file, err = readBounded(part, validators.MaxFileBytes)
		case "current_balance_minor":
			if !includeAccountFields {
				err = errors.New("unexpected multipart field")
			} else {
				var value []byte
				value, err = readBounded(part, 32)
				form.currentBalanceMinor = strings.TrimSpace(string(value))
			}
		default:
			err = errors.New("unexpected multipart field")
		}
		part.Close()
		if err != nil {
			return multipartForm{}, err
		}
	}
	if len(form.file) == 0 {
		return multipartForm{}, errors.New("missing multipart field")
	}
	return form, nil
}

func readBounded(reader io.Reader, maximum int64) ([]byte, error) {
	value, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(value)) > maximum {
		return nil, errors.New("multipart field exceeds bound")
	}
	return value, nil
}

func optionalBalance(value string) (*int64, error) {
	if value == "" {
		return nil, nil
	}
	if strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		return nil, errors.New("balance sign is invalid")
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return nil, errors.New("balance is invalid")
		}
	}
	balance, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, err
	}
	return &balance, nil
}

func previewResponse(preview models.Preview) map[string]any {
	return map[string]any{
		"account_name":         preview.AccountName,
		"existing_account":     preview.ExistingAccount,
		"account_type":         preview.AccountType,
		"mask":                 preview.Mask,
		"currency":             string(preview.Currency),
		"date_from":            preview.DateFrom.Format("2006-01-02"),
		"date_to":              preview.DateTo.Format("2006-01-02"),
		"transaction_count":    len(preview.Transactions),
		"positive_total_minor": strconv.FormatInt(preview.PositiveTotalMinor, 10),
		"outflow_total_minor":  strconv.FormatInt(preview.OutflowTotalMinor, 10),
	}
}
