package attribute_resource

import (
	"github.com/superchalupa/go-redfish/src/ocp/model"
)

type Option func(*service) error

// ApplyOptions will run all of the provided options, you can give options that
// are for this specific service, or you can give base helper options. If you
// give an unknown option, you will get a runtime panic.
func (s *service) ApplyOption(options ...interface{}) error {
	s.Lock()
	defer s.Unlock()
	for _, o := range options {
		var err error
		switch o := o.(type) {
		case Option:
			err = o(s)
		case model.Option:
			err = o(s.Service)
		default:
			panic("Got the wrong kind of option.")
		}

		if err != nil {
			return err
		}
	}
	return nil
}