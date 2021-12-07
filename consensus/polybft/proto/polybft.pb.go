// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.25.0
// 	protoc        v3.12.0
// source: consensus/polybft/proto/polybft.proto

package proto

import (
	proto "github.com/golang/protobuf/proto"
	any "github.com/golang/protobuf/ptypes/any"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// This is a compile-time assertion that a sufficiently up-to-date version
// of the legacy proto package is being used.
const _ = proto.ProtoPackageIsVersion4

type MessageReq_Type int32

const (
	MessageReq_Preprepare  MessageReq_Type = 0
	MessageReq_Prepare     MessageReq_Type = 1
	MessageReq_Commit      MessageReq_Type = 2
	MessageReq_RoundChange MessageReq_Type = 3
)

// Enum value maps for MessageReq_Type.
var (
	MessageReq_Type_name = map[int32]string{
		0: "Preprepare",
		1: "Prepare",
		2: "Commit",
		3: "RoundChange",
	}
	MessageReq_Type_value = map[string]int32{
		"Preprepare":  0,
		"Prepare":     1,
		"Commit":      2,
		"RoundChange": 3,
	}
)

func (x MessageReq_Type) Enum() *MessageReq_Type {
	p := new(MessageReq_Type)
	*p = x
	return p
}

func (x MessageReq_Type) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (MessageReq_Type) Descriptor() protoreflect.EnumDescriptor {
	return file_consensus_polybft_proto_polybft_proto_enumTypes[0].Descriptor()
}

func (MessageReq_Type) Type() protoreflect.EnumType {
	return &file_consensus_polybft_proto_polybft_proto_enumTypes[0]
}

func (x MessageReq_Type) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use MessageReq_Type.Descriptor instead.
func (MessageReq_Type) EnumDescriptor() ([]byte, []int) {
	return file_consensus_polybft_proto_polybft_proto_rawDescGZIP(), []int{0, 0}
}

type MessageReq struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// type is the type of the message
	Type MessageReq_Type `protobuf:"varint,1,opt,name=type,proto3,enum=v1.MessageReq_Type" json:"type,omitempty"`
	// from is the address of the sender
	From string `protobuf:"bytes,2,opt,name=from,proto3" json:"from,omitempty"`
	// seal is the committed seal if message is commit
	Seal string `protobuf:"bytes,3,opt,name=seal,proto3" json:"seal,omitempty"`
	// signature is the crypto signature of the message
	Signature string `protobuf:"bytes,4,opt,name=signature,proto3" json:"signature,omitempty"`
	// view is the view assigned to the message
	View *View `protobuf:"bytes,5,opt,name=view,proto3" json:"view,omitempty"`
	// hash of the locked block
	Digest string `protobuf:"bytes,6,opt,name=digest,proto3" json:"digest,omitempty"`
	// proposal is the rlp encoded block in preprepare messages
	Proposal *any.Any `protobuf:"bytes,7,opt,name=proposal,proto3" json:"proposal,omitempty"`
}

func (x *MessageReq) Reset() {
	*x = MessageReq{}
	if protoimpl.UnsafeEnabled {
		mi := &file_consensus_polybft_proto_polybft_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MessageReq) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MessageReq) ProtoMessage() {}

func (x *MessageReq) ProtoReflect() protoreflect.Message {
	mi := &file_consensus_polybft_proto_polybft_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MessageReq.ProtoReflect.Descriptor instead.
func (*MessageReq) Descriptor() ([]byte, []int) {
	return file_consensus_polybft_proto_polybft_proto_rawDescGZIP(), []int{0}
}

func (x *MessageReq) GetType() MessageReq_Type {
	if x != nil {
		return x.Type
	}
	return MessageReq_Preprepare
}

func (x *MessageReq) GetFrom() string {
	if x != nil {
		return x.From
	}
	return ""
}

func (x *MessageReq) GetSeal() string {
	if x != nil {
		return x.Seal
	}
	return ""
}

func (x *MessageReq) GetSignature() string {
	if x != nil {
		return x.Signature
	}
	return ""
}

func (x *MessageReq) GetView() *View {
	if x != nil {
		return x.View
	}
	return nil
}

func (x *MessageReq) GetDigest() string {
	if x != nil {
		return x.Digest
	}
	return ""
}

func (x *MessageReq) GetProposal() *any.Any {
	if x != nil {
		return x.Proposal
	}
	return nil
}

type View struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Round    uint64 `protobuf:"varint,1,opt,name=round,proto3" json:"round,omitempty"`
	Sequence uint64 `protobuf:"varint,2,opt,name=sequence,proto3" json:"sequence,omitempty"`
}

func (x *View) Reset() {
	*x = View{}
	if protoimpl.UnsafeEnabled {
		mi := &file_consensus_polybft_proto_polybft_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *View) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*View) ProtoMessage() {}

func (x *View) ProtoReflect() protoreflect.Message {
	mi := &file_consensus_polybft_proto_polybft_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use View.ProtoReflect.Descriptor instead.
func (*View) Descriptor() ([]byte, []int) {
	return file_consensus_polybft_proto_polybft_proto_rawDescGZIP(), []int{1}
}

func (x *View) GetRound() uint64 {
	if x != nil {
		return x.Round
	}
	return 0
}

func (x *View) GetSequence() uint64 {
	if x != nil {
		return x.Sequence
	}
	return 0
}

var File_consensus_polybft_proto_polybft_proto protoreflect.FileDescriptor

var file_consensus_polybft_proto_polybft_proto_rawDesc = []byte{
	0x0a, 0x25, 0x63, 0x6f, 0x6e, 0x73, 0x65, 0x6e, 0x73, 0x75, 0x73, 0x2f, 0x70, 0x6f, 0x6c, 0x79,
	0x62, 0x66, 0x74, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x70, 0x6f, 0x6c, 0x79, 0x62, 0x66,
	0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x02, 0x76, 0x31, 0x1a, 0x19, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x61, 0x6e, 0x79,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xa5, 0x02, 0x0a, 0x0a, 0x4d, 0x65, 0x73, 0x73, 0x61,
	0x67, 0x65, 0x52, 0x65, 0x71, 0x12, 0x27, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x0e, 0x32, 0x13, 0x2e, 0x76, 0x31, 0x2e, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65,
	0x52, 0x65, 0x71, 0x2e, 0x54, 0x79, 0x70, 0x65, 0x52, 0x04, 0x74, 0x79, 0x70, 0x65, 0x12, 0x12,
	0x0a, 0x04, 0x66, 0x72, 0x6f, 0x6d, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x66, 0x72,
	0x6f, 0x6d, 0x12, 0x12, 0x0a, 0x04, 0x73, 0x65, 0x61, 0x6c, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x04, 0x73, 0x65, 0x61, 0x6c, 0x12, 0x1c, 0x0a, 0x09, 0x73, 0x69, 0x67, 0x6e, 0x61, 0x74,
	0x75, 0x72, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x73, 0x69, 0x67, 0x6e, 0x61,
	0x74, 0x75, 0x72, 0x65, 0x12, 0x1c, 0x0a, 0x04, 0x76, 0x69, 0x65, 0x77, 0x18, 0x05, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x08, 0x2e, 0x76, 0x31, 0x2e, 0x56, 0x69, 0x65, 0x77, 0x52, 0x04, 0x76, 0x69,
	0x65, 0x77, 0x12, 0x16, 0x0a, 0x06, 0x64, 0x69, 0x67, 0x65, 0x73, 0x74, 0x18, 0x06, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x06, 0x64, 0x69, 0x67, 0x65, 0x73, 0x74, 0x12, 0x30, 0x0a, 0x08, 0x70, 0x72,
	0x6f, 0x70, 0x6f, 0x73, 0x61, 0x6c, 0x18, 0x07, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x41,
	0x6e, 0x79, 0x52, 0x08, 0x70, 0x72, 0x6f, 0x70, 0x6f, 0x73, 0x61, 0x6c, 0x22, 0x40, 0x0a, 0x04,
	0x54, 0x79, 0x70, 0x65, 0x12, 0x0e, 0x0a, 0x0a, 0x50, 0x72, 0x65, 0x70, 0x72, 0x65, 0x70, 0x61,
	0x72, 0x65, 0x10, 0x00, 0x12, 0x0b, 0x0a, 0x07, 0x50, 0x72, 0x65, 0x70, 0x61, 0x72, 0x65, 0x10,
	0x01, 0x12, 0x0a, 0x0a, 0x06, 0x43, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x10, 0x02, 0x12, 0x0f, 0x0a,
	0x0b, 0x52, 0x6f, 0x75, 0x6e, 0x64, 0x43, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x10, 0x03, 0x22, 0x38,
	0x0a, 0x04, 0x56, 0x69, 0x65, 0x77, 0x12, 0x14, 0x0a, 0x05, 0x72, 0x6f, 0x75, 0x6e, 0x64, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x04, 0x52, 0x05, 0x72, 0x6f, 0x75, 0x6e, 0x64, 0x12, 0x1a, 0x0a, 0x08,
	0x73, 0x65, 0x71, 0x75, 0x65, 0x6e, 0x63, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x04, 0x52, 0x08,
	0x73, 0x65, 0x71, 0x75, 0x65, 0x6e, 0x63, 0x65, 0x42, 0x1a, 0x5a, 0x18, 0x2f, 0x63, 0x6f, 0x6e,
	0x73, 0x65, 0x6e, 0x73, 0x75, 0x73, 0x2f, 0x70, 0x6f, 0x6c, 0x79, 0x62, 0x66, 0x74, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_consensus_polybft_proto_polybft_proto_rawDescOnce sync.Once
	file_consensus_polybft_proto_polybft_proto_rawDescData = file_consensus_polybft_proto_polybft_proto_rawDesc
)

func file_consensus_polybft_proto_polybft_proto_rawDescGZIP() []byte {
	file_consensus_polybft_proto_polybft_proto_rawDescOnce.Do(func() {
		file_consensus_polybft_proto_polybft_proto_rawDescData = protoimpl.X.CompressGZIP(file_consensus_polybft_proto_polybft_proto_rawDescData)
	})
	return file_consensus_polybft_proto_polybft_proto_rawDescData
}

var file_consensus_polybft_proto_polybft_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_consensus_polybft_proto_polybft_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_consensus_polybft_proto_polybft_proto_goTypes = []interface{}{
	(MessageReq_Type)(0), // 0: v1.MessageReq.Type
	(*MessageReq)(nil),   // 1: v1.MessageReq
	(*View)(nil),         // 2: v1.View
	(*any.Any)(nil),      // 3: google.protobuf.Any
}
var file_consensus_polybft_proto_polybft_proto_depIdxs = []int32{
	0, // 0: v1.MessageReq.type:type_name -> v1.MessageReq.Type
	2, // 1: v1.MessageReq.view:type_name -> v1.View
	3, // 2: v1.MessageReq.proposal:type_name -> google.protobuf.Any
	3, // [3:3] is the sub-list for method output_type
	3, // [3:3] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_consensus_polybft_proto_polybft_proto_init() }
func file_consensus_polybft_proto_polybft_proto_init() {
	if File_consensus_polybft_proto_polybft_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_consensus_polybft_proto_polybft_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*MessageReq); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_consensus_polybft_proto_polybft_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*View); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_consensus_polybft_proto_polybft_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_consensus_polybft_proto_polybft_proto_goTypes,
		DependencyIndexes: file_consensus_polybft_proto_polybft_proto_depIdxs,
		EnumInfos:         file_consensus_polybft_proto_polybft_proto_enumTypes,
		MessageInfos:      file_consensus_polybft_proto_polybft_proto_msgTypes,
	}.Build()
	File_consensus_polybft_proto_polybft_proto = out.File
	file_consensus_polybft_proto_polybft_proto_rawDesc = nil
	file_consensus_polybft_proto_polybft_proto_goTypes = nil
	file_consensus_polybft_proto_polybft_proto_depIdxs = nil
}
