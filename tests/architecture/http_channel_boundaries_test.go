package architecture

import "testing"

func TestGatewayExportedAPIDoesNotExposeStoreTypes(t *testing.T) {
	assertNoExportedImportSelectors(t, storeImportPath, "internal/gateway")
}

func TestHTTPExportedAPIDoesNotExposeStoreTypes(t *testing.T) {
	assertNoExportedImportSelectors(t, storeImportPath, "internal/http")
}

func TestChannelsExportedAPIDoesNotExposeStoreTypes(t *testing.T) {
	assertNoExportedImportSelectors(t, storeImportPath, "internal/channels")
}
