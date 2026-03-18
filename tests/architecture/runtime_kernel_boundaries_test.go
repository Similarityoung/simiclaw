package architecture

import "testing"

func TestGatewayProductionCodeDoesNotImportStore(t *testing.T) {
	assertNoPackageImport(t, storeImportPath, "internal/gateway")
}

func TestGatewayProductionCodeDoesNotReferenceStoreDB(t *testing.T) {
	assertNoStoreDBReference(t, "internal/gateway")
}

func TestHTTPProductionCodeDoesNotImportStore(t *testing.T) {
	assertNoPackageImport(t, storeImportPath, "internal/http")
}

func TestHTTPProductionCodeDoesNotReferenceStoreDB(t *testing.T) {
	assertNoStoreDBReference(t, "internal/http")
}

func TestRuntimeKernelProductionCodeDoesNotImportStore(t *testing.T) {
	assertNoPackageImport(t, storeImportPath, "internal/runtime/kernel")
}

func TestRuntimeKernelProductionCodeDoesNotReferenceStoreDB(t *testing.T) {
	assertNoStoreDBReference(t, "internal/runtime/kernel")
}
