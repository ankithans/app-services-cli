package kafka

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/redhat-developer/app-services-cli/pkg/cmd/factory"
	"github.com/redhat-developer/app-services-cli/pkg/common/commonerr"
	"github.com/redhat-developer/app-services-cli/pkg/connection"
	"github.com/redhat-developer/app-services-cli/pkg/kafka/kafkaerr"
	"github.com/redhat-developer/app-services-cli/pkg/localize"
	kafkamgmtclient "github.com/redhat-developer/app-services-sdk-go/kafkamgmt/apiv1/client"
)

var (
	validNameRegexp   = regexp.MustCompile(`^[a-z]([-a-z0-9]*[a-z0-9])?$`)
	validSearchRegexp = regexp.MustCompile(`^([a-zA-Z0-9-_%]*[a-zA-Z0-9-_%])?$`)
)

// Validator is a type for validating Kafka configuration values
type Validator struct {
	Localizer  localize.Localizer
	Connection factory.ConnectionFunc
}

// ValidateName validates the proposed name of a Kafka instance
func (v *Validator) ValidateName(val interface{}) error {
	name, ok := val.(string)

	if !ok {
		return commonerr.NewCastError(val, "string")
	}

	if len(name) < 1 || len(name) > 32 {
		return errors.New(v.Localizer.MustLocalize("kafka.validation.name.error.lengthError"))
	}

	matched := validNameRegexp.MatchString(name)

	if matched {
		return nil
	}

	return kafkaerr.InvalidNameError(name)
}

// TransformKafkaRequestListItems modifies fields fields from a list of kafka instances
// The main transformation is appending ":443" to the Bootstrap Server URL
func TransformKafkaRequestListItems(items []kafkamgmtclient.KafkaRequest) []kafkamgmtclient.KafkaRequest {
	for i := range items {
		kafka := items[i]
		kafka = *TransformKafkaRequest(&kafka)
		items[i] = kafka
	}

	return items
}

// TransformKafkaRequest modifies fields from the KafkaRequest payload object
// The main transformation is appending ":443" to the Bootstrap Server URL
func TransformKafkaRequest(kafka *kafkamgmtclient.KafkaRequest) *kafkamgmtclient.KafkaRequest {
	bootstrapHost := kafka.GetBootstrapServerHost()

	if bootstrapHost == "" {
		return kafka
	}

	if !strings.HasSuffix(bootstrapHost, ":443") {
		hostURL := fmt.Sprintf("%v:443", bootstrapHost)
		kafka.SetBootstrapServerHost(hostURL)
	}

	return kafka
}

// ValidateSearchInput validates the text provided to filter the Kafka instances
func (v *Validator) ValidateSearchInput(val interface{}) error {
	search, ok := val.(string)

	if !ok {
		return commonerr.NewCastError(val, "string")
	}

	matched := validSearchRegexp.MatchString(search)

	if matched {
		return nil
	}

	return kafkaerr.InvalidSearchValueError(search)
}

// ValidateNameIsAvailable checks if a kafka instance with the given name already exists
func (v *Validator) ValidateNameIsAvailable(val interface{}) error {
	name, _ := val.(string)

	connection, err := v.Connection(connection.DefaultConfigSkipMasAuth)
	if err != nil {
		return err
	}

	api := connection.API()

	_, httpRes, _ := GetKafkaByName(context.Background(), api.Kafka(), name)

	if httpRes != nil && httpRes.StatusCode == 200 {
		return errors.New(v.Localizer.MustLocalize("kafka.create.error.conflictError", localize.NewEntry("Name", name)))
	}

	if httpRes != nil && httpRes.Body != nil {
		httpRes.Body.Close()
	}

	return nil
}
