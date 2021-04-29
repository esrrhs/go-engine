package common

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"io/ioutil"
)

func LoadProtobuf(filename string) (error, []protoreflect.FileDescriptor) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return err, nil
	}

	fds := &descriptorpb.FileDescriptorSet{}
	err = proto.Unmarshal(b, fds)
	if err != nil {
		return err, nil
	}

	ff, err := protodesc.NewFiles(fds)
	if err != nil {
		return err, nil
	}

	var ret []protoreflect.FileDescriptor
	ff.RangeFiles(func(descriptor protoreflect.FileDescriptor) bool {
		ret = append(ret, descriptor)
		return true
	})

	return nil, ret
}
