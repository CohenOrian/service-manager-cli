/*
 * Copyright 2018 The Service Manager Authors
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 */

package smclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Peripli/service-manager/pkg/web"

	"github.com/Peripli/service-manager-cli/pkg/auth/oidc"

	"github.com/Peripli/service-manager-cli/pkg/auth"
	"github.com/Peripli/service-manager-cli/pkg/errors"
	"github.com/Peripli/service-manager-cli/pkg/httputil"
	"github.com/Peripli/service-manager-cli/pkg/query"
	"github.com/Peripli/service-manager-cli/pkg/types"
)

// Client should be implemented by SM clients
//go:generate counterfeiter . Client
type Client interface {
	GetInfo(*query.Parameters) (*types.Info, error)

	RegisterPlatform(*types.Platform, *query.Parameters) (*types.Platform, error)
	ListPlatforms(*query.Parameters) (*types.Platforms, error)
	UpdatePlatform(string, *types.Platform, *query.Parameters) (*types.Platform, error)
	DeletePlatforms(*query.Parameters) error

	RegisterBroker(*types.Broker, *query.Parameters) (*types.Broker, error)
	ListBrokers(*query.Parameters) (*types.Brokers, error)
	UpdateBroker(string, *types.Broker, *query.Parameters) (*types.Broker, error)
	DeleteBrokers(*query.Parameters) error

	RegisterVisibility(*types.Visibility, *query.Parameters) (*types.Visibility, error)
	ListVisibilities(*query.Parameters) (*types.Visibilities, error)
	UpdateVisibility(string, *types.Visibility, *query.Parameters) (*types.Visibility, error)
	DeleteVisibilities(*query.Parameters) error

	ListOfferings(*query.Parameters) (*types.ServiceOfferings, error)

	Label(string, string, *types.LabelChanges, *query.Parameters) error

	// Call makes HTTP request to the Service Manager server with authentication.
	// It should be used only in case there is no already implemented method for such an operation
	Call(method string, smpath string, body io.Reader, q *query.Parameters) (*http.Response, error)
}

type serviceManagerClient struct {
	config     *ClientConfig
	httpClient auth.Client
}

// NewClientWithAuth returns new SM Client configured with the provided configuration
func NewClientWithAuth(httpClient auth.Client, config *ClientConfig) (Client, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	client := &serviceManagerClient{config: config, httpClient: httpClient}
	info, err := client.GetInfo(nil)
	if err != nil {
		return nil, err
	}

	authOptions := &auth.Options{
		IssuerURL:    info.TokenIssuerURL,
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		SSLDisabled:  config.SSLDisabled,
	}
	var authStrategy auth.Authenticator
	authStrategy, authOptions, err = oidc.NewOpenIDStrategy(authOptions)

	if err != nil {
		return nil, err
	}

	token, err := auth.GetToken(authOptions, authStrategy)
	if err != nil {
		return nil, err
	}
	authClient := oidc.NewClient(authOptions, token)
	client = &serviceManagerClient{config: config, httpClient: authClient}

	return client, nil
}

// NewClient returns new SM client which will use the http client provided to make calls
func NewClient(httpClient auth.Client, URL string) Client {
	return &serviceManagerClient{config: &ClientConfig{URL: URL}, httpClient: httpClient}
}

func (client *serviceManagerClient) GetInfo(q *query.Parameters) (*types.Info, error) {
	response, err := client.Call(http.MethodGet, web.InfoURL, nil, q)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		return nil, errors.ResponseError{StatusCode: response.StatusCode}
	}

	info := types.DefaultInfo
	err = httputil.UnmarshalResponse(response, &info)
	if err != nil {
		return nil, err
	}

	return &info, nil
}

// RegisterPlatform registers a platform in the service manager
func (client *serviceManagerClient) RegisterPlatform(platform *types.Platform, q *query.Parameters) (*types.Platform, error) {
	var newPlatform *types.Platform
	err := client.register(platform, web.PlatformsURL, q, &newPlatform)
	if err != nil {
		return nil, err
	}
	return newPlatform, nil
}

// RegisterBroker registers a broker in the service manager
func (client *serviceManagerClient) RegisterBroker(broker *types.Broker, q *query.Parameters) (*types.Broker, error) {
	var newBroker *types.Broker
	err := client.register(broker, web.ServiceBrokersURL, q, &newBroker)
	if err != nil {
		return nil, err
	}
	return newBroker, nil
}

// RegisterVisibility registers a visibility in the service manager
func (client *serviceManagerClient) RegisterVisibility(visibility *types.Visibility, q *query.Parameters) (*types.Visibility, error) {
	var newVisibility *types.Visibility
	err := client.register(visibility, web.VisibilitiesURL, q, &newVisibility)
	if err != nil {
		return nil, err
	}
	return newVisibility, nil
}

func (client *serviceManagerClient) register(resource interface{}, url string, q *query.Parameters, result interface{}) error {
	requestBody, err := json.Marshal(resource)
	if err != nil {
		return err
	}

	buffer := bytes.NewBuffer(requestBody)
	response, err := client.Call(http.MethodPost, url, buffer, q)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusCreated {
		return errors.ResponseError{StatusCode: response.StatusCode}
	}

	return httputil.UnmarshalResponse(response, &result)
}

// ListBrokers returns brokers registered in the Service Manager satisfying provided queries
func (client *serviceManagerClient) ListBrokers(q *query.Parameters) (*types.Brokers, error) {
	brokers := &types.Brokers{}
	err := client.list(brokers, web.ServiceBrokersURL, q)

	return brokers, err
}

// ListPlatforms returns platforms registered in the Service Manager satisfying provided queries
func (client *serviceManagerClient) ListPlatforms(q *query.Parameters) (*types.Platforms, error) {
	platforms := &types.Platforms{}
	err := client.list(platforms, web.PlatformsURL, q)

	return platforms, err
}

func (client *serviceManagerClient) ListVisibilities(q *query.Parameters) (*types.Visibilities, error) {
	visibilities := &types.Visibilities{}
	err := client.list(visibilities, web.VisibilitiesURL, q)
	return visibilities, err
}

// ListOfferings returns service offerings satisfying provided queries
func (client *serviceManagerClient) ListOfferings(q *query.Parameters) (*types.ServiceOfferings, error) {
	serviceOfferings := &types.ServiceOfferings{}
	err := client.list(serviceOfferings, web.ServiceOfferingsURL, q)
	if err != nil {
		return nil, err
	}
	for i, so := range serviceOfferings.ServiceOfferings {
		plans := &types.ServicePlans{}
		err := client.list(plans, web.ServicePlansURL, &query.Parameters{
			FieldQuery:    []string{fmt.Sprintf("service_offering_id = %s", so.ID)},
			GeneralParams: q.GeneralParams,
		})
		if err != nil {
			return nil, err
		}
		serviceOfferings.ServiceOfferings[i].Plans = plans.ServicePlans

		broker := &types.Broker{}
		err = client.list(broker, web.ServiceBrokersURL+"/"+so.BrokerID, &query.Parameters{
			GeneralParams: q.GeneralParams,
		})
		if err != nil {
			return nil, err
		}

		serviceOfferings.ServiceOfferings[i].BrokerName = broker.Name
	}
	return serviceOfferings, nil
}

func (client *serviceManagerClient) list(result interface{}, url string, q *query.Parameters) error {
	resp, err := client.Call(http.MethodGet, url, nil, q)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return errors.ResponseError{StatusCode: resp.StatusCode}
	}

	return httputil.UnmarshalResponse(resp, &result)
}

func (client *serviceManagerClient) DeleteBrokers(q *query.Parameters) error {
	return client.delete(web.ServiceBrokersURL, q)
}

// DeleteBroker deletes a broker with given id from service manager
func (client *serviceManagerClient) DeleteBroker(id string, q *query.Parameters) error {
	return client.delete(web.ServiceBrokersURL+"/"+id, q)
}

func (client *serviceManagerClient) DeletePlatforms(q *query.Parameters) error {
	return client.delete(web.PlatformsURL, q)
}

// DeletePlatform deletes a platform with given id from service manager
func (client *serviceManagerClient) DeletePlatform(id string, q *query.Parameters) error {
	return client.delete(web.PlatformsURL+"/"+id, q)
}

func (client *serviceManagerClient) DeleteVisibilities(q *query.Parameters) error {
	return client.delete(web.VisibilitiesURL, q)
}

// DeleteVisibility deletes a visibility with given id from service manager
func (client *serviceManagerClient) DeleteVisibility(id string, q *query.Parameters) error {
	return client.delete(web.VisibilitiesURL+"/"+id, q)
}

func (client *serviceManagerClient) delete(url string, q *query.Parameters) error {
	resp, err := client.Call(http.MethodDelete, url, nil, q)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return errors.ResponseError{StatusCode: resp.StatusCode}
	}

	return nil
}

func (client *serviceManagerClient) UpdateBroker(id string, updatedBroker *types.Broker, q *query.Parameters) (*types.Broker, error) {
	result := &types.Broker{}
	err := client.update(updatedBroker, web.ServiceBrokersURL, id, q, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (client *serviceManagerClient) UpdatePlatform(id string, updatedPlatform *types.Platform, q *query.Parameters) (*types.Platform, error) {
	result := &types.Platform{}
	err := client.update(updatedPlatform, web.PlatformsURL, id, q, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (client *serviceManagerClient) UpdateVisibility(id string, updatedVisibility *types.Visibility, q *query.Parameters) (*types.Visibility, error) {
	result := &types.Visibility{}
	err := client.update(updatedVisibility, web.VisibilitiesURL, id, q, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (client *serviceManagerClient) update(resource interface{}, url string, id string, q *query.Parameters, result interface{}) error {
	requestBody, err := json.Marshal(resource)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer(requestBody)
	resp, err := client.Call(http.MethodPatch, url+"/"+id, buffer, q)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return errors.ResponseError{StatusCode: resp.StatusCode}
	}

	return httputil.UnmarshalResponse(resp, &result)
}

func (client *serviceManagerClient) Label(url string, id string, change *types.LabelChanges, q *query.Parameters) error {
	requestBody, err := json.Marshal(change)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer(requestBody)
	response, err := client.Call(http.MethodPatch, url+"/"+id, buffer, q)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusOK {
		return errors.ResponseError{StatusCode: response.StatusCode}
	}

	return nil
}

func (client *serviceManagerClient) Call(method string, smpath string, body io.Reader, q *query.Parameters) (*http.Response, error) {
	fullURL := httputil.NormalizeURL(client.config.URL) + buildURL(smpath, q)

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		respErr := errors.ResponseError{
			URL:        fullURL,
			StatusCode: resp.StatusCode,
		}

		respContent := make(map[string]interface{})
		if err := httputil.UnmarshalResponse(resp, &respContent); err != nil {
			return resp, respErr
		}

		if errorMessage, ok := respContent["error"].(string); ok {
			respErr.ErrorMessage = errorMessage
		}

		if description, ok := respContent["description"].(string); ok {
			respErr.Description = description
		}

		return nil, respErr
	}

	return resp, nil
}

func buildURL(baseURL string, q *query.Parameters) string {
	queryParams := q.Encode()
	if queryParams == "" {
		return baseURL
	}
	return baseURL + "?" + queryParams
}
