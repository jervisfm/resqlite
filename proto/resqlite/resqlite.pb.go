// Code generated by protoc-gen-go. DO NOT EDIT.
// source: resqlite.proto

/*
Package proto_resqlite is a generated protocol buffer package.

It is generated from these files:
	resqlite.proto

It has these top-level messages:
	SqlCommandRequest
	SqlCommandResponse
*/
package proto_resqlite

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

type SqlCommandRequest struct {
	// Query
	Query string `protobuf:"bytes,1,opt,name=query" json:"query,omitempty"`
}

func (m *SqlCommandRequest) Reset()                    { *m = SqlCommandRequest{} }
func (m *SqlCommandRequest) String() string            { return proto.CompactTextString(m) }
func (*SqlCommandRequest) ProtoMessage()               {}
func (*SqlCommandRequest) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{0} }

func (m *SqlCommandRequest) GetQuery() string {
	if m != nil {
		return m.Query
	}
	return ""
}

type SqlCommandResponse struct {
	// Response
	Response string `protobuf:"bytes,1,opt,name=response" json:"response,omitempty"`
}

func (m *SqlCommandResponse) Reset()                    { *m = SqlCommandResponse{} }
func (m *SqlCommandResponse) String() string            { return proto.CompactTextString(m) }
func (*SqlCommandResponse) ProtoMessage()               {}
func (*SqlCommandResponse) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{1} }

func (m *SqlCommandResponse) GetResponse() string {
	if m != nil {
		return m.Response
	}
	return ""
}

func init() {
	proto.RegisterType((*SqlCommandRequest)(nil), "proto_resqlite.SqlCommandRequest")
	proto.RegisterType((*SqlCommandResponse)(nil), "proto_resqlite.SqlCommandResponse")
}

func init() { proto.RegisterFile("resqlite.proto", fileDescriptor0) }

var fileDescriptor0 = []byte{
	// 178 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0xe2, 0x2b, 0x4a, 0x2d, 0x2e,
	0xcc, 0xc9, 0x2c, 0x49, 0xd5, 0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x17, 0xe2, 0x03, 0x53, 0xf1, 0x30,
	0x51, 0x25, 0x4d, 0x2e, 0xc1, 0xe0, 0xc2, 0x1c, 0xe7, 0xfc, 0xdc, 0xdc, 0xc4, 0xbc, 0x94, 0xa0,
	0xd4, 0xc2, 0xd2, 0xd4, 0xe2, 0x12, 0x21, 0x11, 0x2e, 0xd6, 0xc2, 0xd2, 0xd4, 0xa2, 0x4a, 0x09,
	0x46, 0x05, 0x46, 0x0d, 0xce, 0x20, 0x08, 0x47, 0xc9, 0x80, 0x4b, 0x08, 0x59, 0x69, 0x71, 0x41,
	0x7e, 0x5e, 0x71, 0xaa, 0x90, 0x14, 0x17, 0x47, 0x11, 0x94, 0x0d, 0x55, 0x0e, 0xe7, 0x1b, 0xa5,
	0x72, 0x71, 0x04, 0xa5, 0x06, 0x83, 0x2d, 0x12, 0x8a, 0xe4, 0xe2, 0x0b, 0x4e, 0xcd, 0x4b, 0x41,
	0x98, 0x20, 0xa4, 0xa8, 0x87, 0xea, 0x16, 0x3d, 0x0c, 0x87, 0x48, 0x29, 0xe1, 0x53, 0x02, 0xb1,
	0x44, 0x89, 0xc1, 0xc9, 0x94, 0x4b, 0x29, 0x39, 0x3f, 0x57, 0x2f, 0x3d, 0xb3, 0x24, 0xa3, 0x34,
	0x49, 0x0f, 0xd5, 0xc3, 0x70, 0xae, 0x13, 0x2f, 0xcc, 0x29, 0x01, 0x20, 0xf1, 0x00, 0xc6, 0x24,
	0x36, 0xb0, 0x02, 0x63, 0x40, 0x00, 0x00, 0x00, 0xff, 0xff, 0xd8, 0xf4, 0x86, 0xa5, 0x23, 0x01,
	0x00, 0x00,
}
