package main

import (
	"fmt"
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
	client, adapter, err := setupClient("TenantID",
		"ClientID", "ClientSecret",
		[]string{"https://graph.microsoft.com/.default"})
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
		fmt.Printf("%s %v %v %v\n", *app.GetDisplayName(), isOlderThanThreeMonths, hasSignIn, hasNotExpired)
	}
}

func getApplications(client *msgraph.GraphServiceClient, adapter *msgraph.GraphRequestAdapter) ([]models.Applicationable, error) {
	enableAdvancedQueryCapabilities := true
	queryParams := applications.ApplicationsRequestBuilderGetQueryParameters{
		Select:  []string{"appId", "displayName", "createdDateTime", "tags"},
		Orderby: []string{"displayName"},
		Count:   &enableAdvancedQueryCapabilities,
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
