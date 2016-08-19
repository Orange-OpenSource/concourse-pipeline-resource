package helpers

import (
	"fmt"
	"io"
	"io/ioutil"

	"github.com/concourse/atc"
	"github.com/concourse/fly/template"
	"github.com/mitchellh/mapstructure"
	"github.com/onsi/gomega/gexec"
	"github.com/robdimsdale/concourse-pipeline-resource/concourse/api"
	"gopkg.in/yaml.v2"
)

//go:generate counterfeiter . PipelineSetter

type PipelineSetter interface {
	SetPipeline(
		pipelineName string,
		configPath string,
		templateVariables template.Variables,
		templateVariablesFiles []string,
	) error
}

type pipelineSetter struct {
	client       api.Client
	configDiffer ConfigDiffer
}

func NewPipelineSetter(client api.Client, configDiffer ConfigDiffer) PipelineSetter {
	return &pipelineSetter{
		client:       client,
		configDiffer: configDiffer,
	}
}

func (p pipelineSetter) SetPipeline(
	pipelineName string,
	configPath string,
	templateVariables template.Variables,
	templateVariablesFiles []string,
) error {
	newConfig, err := p.newConfig(
		configPath,
		templateVariablesFiles,
		templateVariables,
	)
	if err != nil {
		return err
	}

	existingConfig, _, existingConfigVersion, err :=
		p.client.PipelineConfig(pipelineName)
	if err != nil {
		return err
	}

	p.configDiffer.Diff(existingConfig, newConfig)

	err = p.client.SetPipelineConfig(
		pipelineName,
		existingConfigVersion,
		newConfig,
	)
	if err != nil {
		return err
	}

	return nil
}

func (p pipelineSetter) newConfig(
	configPath string,
	templateVariablesFiles []string,
	templateVariables template.Variables,
) (atc.Config, error) {
	configFile, err := ioutil.ReadFile(configPath)
	if err != nil {
		return atc.Config{}, err
	}

	var resultVars template.Variables

	for _, path := range templateVariablesFiles {
		fileVars, templateErr := template.LoadVariablesFromFile(string(path))
		if templateErr != nil {
			return atc.Config{}, templateErr
		}

		resultVars = resultVars.Merge(fileVars)
	}

	resultVars = resultVars.Merge(templateVariables)

	configFile, err = template.Evaluate(configFile, resultVars)
	if err != nil {
		return atc.Config{}, err
	}

	var configStructure interface{}
	err = yaml.Unmarshal(configFile, &configStructure)
	if err != nil {
		return atc.Config{}, err
	}

	var newConfig atc.Config
	msConfig := &mapstructure.DecoderConfig{
		Result:           &newConfig,
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			atc.SanitizeDecodeHook,
			atc.VersionConfigDecodeHook,
		),
	}

	decoder, err := mapstructure.NewDecoder(msConfig)
	if err != nil {
		return atc.Config{}, err
	}

	if err := decoder.Decode(configStructure); err != nil {
		return atc.Config{}, err
	}

	return newConfig, nil
}

func diff(to io.Writer, existingConfig atc.Config, newConfig atc.Config) {
	indent := gexec.NewPrefixedWriter("  ", to)

	groupDiffs := diffIndices(GroupIndex(existingConfig.Groups), GroupIndex(newConfig.Groups))
	if len(groupDiffs) > 0 {
		fmt.Fprintln(to, "groups:")

		for _, diff := range groupDiffs {
			diff.Render(indent, "group")
		}
	}

	resourceDiffs := diffIndices(ResourceIndex(existingConfig.Resources), ResourceIndex(newConfig.Resources))
	if len(resourceDiffs) > 0 {
		fmt.Fprintln(to, "resources:")

		for _, diff := range resourceDiffs {
			diff.Render(indent, "resource")
		}
	}

	resourceTypeDiffs := diffIndices(ResourceTypeIndex(existingConfig.ResourceTypes), ResourceTypeIndex(newConfig.ResourceTypes))
	if len(resourceTypeDiffs) > 0 {
		fmt.Fprintln(to, "resource types:")

		for _, diff := range resourceTypeDiffs {
			diff.Render(indent, "resource type")
		}
	}

	jobDiffs := diffIndices(JobIndex(existingConfig.Jobs), JobIndex(newConfig.Jobs))
	if len(jobDiffs) > 0 {
		fmt.Fprintln(to, "jobs:")

		for _, diff := range jobDiffs {
			diff.Render(indent, "job")
		}
	}
}
