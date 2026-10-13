package admin

import (
 "net/http"
 "net/http/httptest"
 "testing"

 "github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/services"
)

func TestUSBDownloadRequiresAuthenticatedSession(t *testing.T) {
 c,err:=NewCredential("temporary-admin-password-123456")
 if err!=nil{t.Fatal(err)}
 handler:=New(c,services.New()).Handler()
 for _,endpoint:=range []string{
  "/api/storage/list?partition=sda1&path=..",
  "/api/storage/file?partition=sda1&path=private",
 } {
  req:=httptest.NewRequest(http.MethodGet,endpoint,nil)
  rr:=httptest.NewRecorder()
  handler.ServeHTTP(rr,req)
  if rr.Code!=http.StatusUnauthorized {t.Fatalf("%s accessible without authentication: %d",endpoint,rr.Code)}
  if rr.Header().Get("X-Content-Type-Options")!="nosniff" {t.Fatalf("missing security header")}
 }
}
