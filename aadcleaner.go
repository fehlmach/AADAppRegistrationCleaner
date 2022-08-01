package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	kiota "github.com/microsoft/kiota-authentication-azure-go"
	msgraph "github.com/microsoftgraph/msgraph-beta-sdk-go"
	"github.com/microsoftgraph/msgraph-beta-sdk-go/applications"
	"github.com/microsoftgraph/msgraph-beta-sdk-go/auditlogs/signins"
	"github.com/microsoftgraph/msgraph-beta-sdk-go/models"
	//msgraphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
)

func main() {
	os.Setenv("TENANT_ID", "")
	os.Setenv("CLIENT_ID", "")
	os.Setenv("CLIENT_SECRET", "")
	os.Setenv("REPORT_ONLY", "true")
	client, adapter, err := setupClient(os.Getenv("TENANT_ID"), os.Getenv("CLIENT_ID"),
		os.Getenv("CLIENT_SECRET"), []string{"https://graph.microsoft.com/.default"})
	if err != nil {
		return
	}

	apps, err := getApplications(client, adapter)
	if err != nil {
		return
	}
	for _, app := range apps {
		isOlderThanThreeMonths := app.GetCreatedDateTime().Before(time.Now().AddDate(0, -3, 0))
		signIns, err := getSignIns(client, adapter, *app.GetAppId())
		if err != nil {
			continue
		}
		hasSignIn := len(signIns) != 0
		hasNotExpired := false
		for _, tag := range app.GetTags() {
			if strings.HasPrefix(tag, "expireOn : ") {
				expiresOn, err := time.Parse("2006-01-02", strings.TrimPrefix(tag, "expireOn : "))
				if err != nil {
					fmt.Printf("Was not able to parse date: %v", err)
					continue
				}
				hasNotExpired = expiresOn.After(time.Now())
			}
		}
		willBeDeleted := isOlderThanThreeMonths && !hasSignIn && !hasNotExpired
		fmt.Printf("DisplayName=%s isOlderThanThreeMonths=%v hasSignIns=%v hasNotExpired=%v willBeDeleted=%v\n", *app.GetDisplayName(), isOlderThanThreeMonths, hasSignIn, hasNotExpired, willBeDeleted)
		if os.Getenv("REPORT_ONLY") == "false" && willBeDeleted {
			err := client.ApplicationsById(*app.GetId()).Delete()
			if err != nil {
				fmt.Printf("Was not able to delete application %v with id=%v: %v", *app.GetDisplayName(),
					*app.GetId(), err)
			}
		}
	}
}

func getApplications(client *msgraph.GraphServiceClient, adapter *msgraph.GraphRequestAdapter) ([]models.Applicationable, error) {
	enableAdvancedQueryCapabilities := true
	filter := "displayName eq 'ipt-app-registration-cleaner'"
	queryParams := applications.ApplicationsRequestBuilderGetQueryParameters{
		Select:  []string{"id", "appId", "displayName", "createdDateTime", "tags"},
		Orderby: []string{"displayName"},
		Count:   &enableAdvancedQueryCapabilities,
		Filter:  &filter,
	}
	reqConfig := applications.ApplicationsRequestBuilderGetRequestConfiguration{
		QueryParameters: &queryParams,
		Headers:         map[string]string{"ConsistencyLevel": "eventual"},
	}
	appResult, err := client.Applications().GetWithRequestConfigurationAndResponseHandler(&reqConfig, nil)
	if err != nil {
		fmt.Printf("Error getting applications: %v\n", err)
		return nil, err
	}

	pageIterator, err := NewPageIterator(appResult, adapter, models.CreateApplicationCollectionResponseFromDiscriminatorValue)
	if err != nil {
		fmt.Printf("PageIterator error: %v\n", err)
		return nil, err
	}
	apps := []models.Applicationable{}
	pageIterator.Iterate(func(pageItem interface{}) bool {
		apps = append(apps, pageItem.(models.Applicationable))
		return true
	})
	return apps, nil
}

func getSignIns(client *msgraph.GraphServiceClient, adapter *msgraph.GraphRequestAdapter, appId string) ([]models.SignInable, error) {
	returnTop1 := int32(1)
	filter := fmt.Sprintf("signInEventTypes/any(t: t eq 'interactiveUser' or t eq 'nonInteractiveUser' or t eq 'servicePrincipal' or t eq 'managedIdentity') and appId eq '%s'", appId)
	queryParams := signins.SignInsRequestBuilderGetQueryParameters{
		Top:     &returnTop1,
		Filter:  &filter,
		Orderby: []string{"createdDateTime desc"},
	}
	reqConfig := signins.SignInsRequestBuilderGetRequestConfiguration{
		QueryParameters: &queryParams,
	}

	signInResult, err := client.AuditLogs().SignIns().GetWithRequestConfigurationAndResponseHandler(&reqConfig, nil)
	if err != nil {
		fmt.Printf("Error getting signIns of application %v: %v\n", appId, err)
		return nil, err
	}
	pageIterator, err := NewPageIterator(signInResult, adapter, models.CreateSignInCollectionResponseFromDiscriminatorValue)
	if err != nil {
		fmt.Printf("PageIterator error: %v\n", err)
		return nil, err
	}
	signIns := []models.SignInable{}
	pageIterator.Iterate(func(pageItem interface{}) bool {
		signIns = append(signIns, pageItem.(models.SignInable))
		return true
	})
	return signIns, nil
}

func setupClient(tenantId, clientId, clientSecret string, scope []string) (*msgraph.GraphServiceClient, *msgraph.GraphRequestAdapter, error) {
	cred, err := azidentity.NewClientSecretCredential(tenantId, clientId, clientSecret, nil)
	if err != nil {
		fmt.Printf("Error creating credentials: %v\n", err)
		return nil, nil, err
	}
	auth, err := kiota.NewAzureIdentityAuthenticationProviderWithScopes(cred, scope)
	if err != nil {
		fmt.Printf("Error authentication provider: %v\n", err)
		return nil, nil, err
	}
	adapter, err := msgraph.NewGraphRequestAdapter(auth)
	if err != nil {
		fmt.Printf("Error creating adapter: %v\n", err)
		return nil, nil, err
	}
	return msgraph.NewGraphServiceClient(adapter), adapter, nil
}
