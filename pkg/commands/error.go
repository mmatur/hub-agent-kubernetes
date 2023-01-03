/*
Copyright (C) 2022-2023 Traefik Labs

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

package commands

import (
	"errors"

	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type reportErrorType string

const (
	reportErrorTypeInternalError      reportErrorType = "internal-error"
	reportErrorTypeUnsupportedCommand reportErrorType = "unsupported-command"
	reportErrorTypeIngressNotFound    reportErrorType = "ingress-not-found"
	reportErrorTypeACPNotFound        reportErrorType = "acp-not-found"
)

func newErrorReport(commandID string, err error) *platform.CommandExecutionReport {
	var statusErr *kerror.StatusError
	if !errors.As(err, &statusErr) {
		return newInternalErrorReport(commandID, err)
	}

	if statusErr.Status().Reason == metav1.StatusReasonNotFound {
		if statusErr.Status().Details == nil {
			return newInternalErrorReport(commandID, err)
		}

		// Resource kind is of the singular version on listers while plural version on api calls. We want
		// to handle both cases similarly.
		switch statusErr.Status().Details.Kind {
		case "ingress", "ingresses":
			return newErrorReportWithType(commandID, reportErrorTypeIngressNotFound)
		case "ingressroute", "ingressroutes":
			return newErrorReportWithType(commandID, reportErrorTypeIngressNotFound)
		case "accesscontrolpolicy", "accesscontrolpolicies":
			return newErrorReportWithType(commandID, reportErrorTypeACPNotFound)
		}
	}

	return newInternalErrorReport(commandID, err)
}

func newErrorReportWithType(commandID string, typ reportErrorType) *platform.CommandExecutionReport {
	return platform.NewErrorCommandExecutionReport(commandID, platform.CommandExecutionReportError{
		Type: string(typ),
	})
}

func newInternalErrorReport(commandID string, err error) *platform.CommandExecutionReport {
	return platform.NewErrorCommandExecutionReport(commandID, platform.CommandExecutionReportError{
		Type: string(reportErrorTypeInternalError),
		Data: err.Error(),
	})
}
