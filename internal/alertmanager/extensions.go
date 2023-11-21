package alertmanager

import (
	"errors"

	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/protobuf/proto"
)

type validatorProto interface {
	validation.Validator
	proto.Message
}

type VisitF[T validatorProto] func(protoSlice []T)

var _ validatorProto = &DiscordConfig{}
var _ validatorProto = &EmailConfig{}
var _ validatorProto = &PagerdutyConfig{}
var _ validatorProto = &SlackConfig{}
var _ validatorProto = &WebhookConfig{}
var _ validatorProto = &OpsGenieConfig{}
var _ validatorProto = &WechatConfig{}
var _ validatorProto = &PushoverConfig{}
var _ validatorProto = &VictorOpsConfig{}
var _ validatorProto = &SNSConfig{}
var _ validatorProto = &TelegramConfig{}
var _ validatorProto = &WebexConfig{}
var _ validatorProto = &MSTeamsConfig{}

func asValidatorProtoSlice[T validatorProto](v []T) []validatorProto {
	s := make([]validatorProto, len(v))
	for i, x := range v {
		s[i] = x
	}
	return s
}

func visitEach(r *Receiver, cb VisitF[validatorProto]) {
	cb(asValidatorProtoSlice(r.DiscordConfigs))
	cb(asValidatorProtoSlice(r.EmailConfigs))
	cb(asValidatorProtoSlice(r.PagerdutyConfigs))
	cb(asValidatorProtoSlice(r.SlackConfigs))
	cb(asValidatorProtoSlice(r.WebhookConfigs))
	cb(asValidatorProtoSlice(r.OpsgenieConfigs))
	cb(asValidatorProtoSlice(r.WechatConfigs))
	cb(asValidatorProtoSlice(r.PushoverConfigs))
	cb(asValidatorProtoSlice(r.VictoropsConfigs))
	cb(asValidatorProtoSlice(r.SnsConfigs))
	cb(asValidatorProtoSlice(r.TelegramConfigs))
	cb(asValidatorProtoSlice(r.WebexConfigs))
	cb(asValidatorProtoSlice(r.MsteamsConfigs))
}

func (r *Receiver) Validate() error {
	i := 0
	visitEach(r, func(protoSlice []validatorProto) {
		i += len(protoSlice)
	})

	if i == 0 {
		return validation.Error("Receiver must contain at least one receiver definition")
	}
	errs := []error{}
	visitEach(r, func(protoSlice []validatorProto) {
		for _, proto := range protoSlice {
			if err := proto.Validate(); err != nil {
				errs = append(errs, err)
			}
		}
	})

	if err := errors.Join(errs...); err != nil {
		return err
	}
	return nil
}

func (d *DiscordConfig) Validate() error {
	return nil
}

func (e *EmailConfig) Validate() error {
	return nil
}

func (p *PagerdutyConfig) Validate() error {
	return nil
}

func (s *SlackConfig) Validate() error {
	return nil
}

func (w *WebhookConfig) Validate() error {
	return nil
}

func (o *OpsGenieConfig) Validate() error {
	return nil
}

func (w *WechatConfig) Validate() error {
	return nil
}

func (p *PushoverConfig) Validate() error {
	return nil
}

func (v *VictorOpsConfig) Validate() error {
	return nil
}

func (s *SNSConfig) Validate() error {
	return nil
}

func (t *TelegramConfig) Validate() error {
	return nil
}

func (w *WebexConfig) Validate() error {
	return nil
}

func (m *MSTeamsConfig) Validate() error {
	return nil
}
