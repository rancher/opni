package controllers

import (
	logcontrollers "github.com/banzaicloud/logging-operator/controllers/logging"
	ctrl "sigs.k8s.io/controller-runtime"
)

type LoggingReconciler logcontrollers.LoggingReconciler

// Implement test.Reconciler for our test environment
func (r *LoggingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.Log = mgr.GetLogger().WithName("Logging")
	return logcontrollers.SetupLoggingWithManager(mgr, r.Log).Complete(
		(*logcontrollers.LoggingReconciler)(r))
}
