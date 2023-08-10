package ast

import (
	ast_pb "github.com/txpull/protos/dist/go/ast"
	"github.com/txpull/solgo/parser"
)

type ParameterList struct {
	*ASTBuilder

	Id             int64              `json:"id"`
	NodeType       ast_pb.NodeType    `json:"node_type"`
	Src            SrcNode            `json:"src"`
	Parameters     []*Parameter       `json:"parameters"`
	ParameterTypes []*TypeDescription `json:"parameter_types"`
}

func NewParameterList(b *ASTBuilder) *ParameterList {
	return &ParameterList{
		ASTBuilder:     b,
		Id:             b.GetNextID(),
		NodeType:       ast_pb.NodeType_PARAMETER_LIST,
		Parameters:     make([]*Parameter, 0),
		ParameterTypes: make([]*TypeDescription, 0),
	}
}

// SetReferenceDescriptor sets the reference descriptions of the ParameterList node.
func (p *ParameterList) SetReferenceDescriptor(refId int64, refDesc *TypeDescription) bool {
	return false
}

func (p *ParameterList) GetId() int64 {
	return p.Id
}

func (p *ParameterList) GetType() ast_pb.NodeType {
	return p.NodeType
}

func (p *ParameterList) GetSrc() SrcNode {
	return p.Src
}

func (p *ParameterList) GetTypeDescription() *TypeDescription {
	return nil
}

func (p *ParameterList) GetParameters() []*Parameter {
	return p.Parameters
}

func (p *ParameterList) GetParameterTypes() []*TypeDescription {
	return p.ParameterTypes
}

func (p *ParameterList) GetNodes() []Node[NodeType] {
	toReturn := make([]Node[NodeType], 0)

	for _, parameter := range p.GetParameters() {
		toReturn = append(toReturn, parameter)
	}

	return toReturn
}

func (p *ParameterList) ToProto() *ast_pb.ParameterList {
	toReturn := &ast_pb.ParameterList{
		Id:       p.GetId(),
		NodeType: p.GetType(),
		Src:      p.GetSrc().ToProto(),
	}

	for _, parameter := range p.GetParameters() {
		toReturn.Parameters = append(
			toReturn.Parameters,
			parameter.ToProto().(*ast_pb.Parameter),
		)
	}

	return toReturn
}

func (p *ParameterList) Parse(unit *SourceUnit[Node[ast_pb.SourceUnit]], fNode Node[NodeType], ctx parser.IParameterListContext) {
	p.Src = SrcNode{
		Id:          p.GetNextID(),
		Line:        int64(ctx.GetStart().GetLine()),
		Column:      int64(ctx.GetStart().GetColumn()),
		Start:       int64(ctx.GetStart().GetStart()),
		End:         int64(ctx.GetStop().GetStop()),
		Length:      int64(ctx.GetStop().GetStop() - ctx.GetStart().GetStart() + 1),
		ParentIndex: fNode.GetId(),
	}

	// No need to move forwards as there are no parameters to parse in this context.
	if ctx == nil || ctx.IsEmpty() {
		return
	}

	for _, paramCtx := range ctx.AllParameterDeclaration() {
		param := NewParameter(p.ASTBuilder)
		param.Parse(unit, fNode, p, paramCtx.(*parser.ParameterDeclarationContext))
		p.Parameters = append(p.Parameters, param)
		p.ParameterTypes = append(p.ParameterTypes, param.TypeName.TypeDescription)
	}
}

func (p *ParameterList) ParseEventParameters(unit *SourceUnit[Node[ast_pb.SourceUnit]], eNode Node[NodeType], ctx []parser.IEventParameterContext) {
	p.Src = eNode.GetSrc()
	p.Src.ParentIndex = eNode.GetId()

	for _, paramCtx := range ctx {
		param := NewParameter(p.ASTBuilder)
		param.ParseEventParameter(unit, eNode, p, paramCtx)
		p.Parameters = append(p.Parameters, param)
		p.ParameterTypes = append(p.ParameterTypes, param.TypeName.TypeDescription)
	}
}

func (p *ParameterList) ParseErrorParameters(unit *SourceUnit[Node[ast_pb.SourceUnit]], eNode Node[NodeType], ctx []parser.IErrorParameterContext) {
	p.Src = eNode.GetSrc()
	p.Src.ParentIndex = eNode.GetId()

	for _, paramCtx := range ctx {
		param := NewParameter(p.ASTBuilder)
		param.ParseErrorParameter(unit, eNode, p, paramCtx)
		p.Parameters = append(p.Parameters, param)
		p.ParameterTypes = append(p.ParameterTypes, param.TypeName.TypeDescription)
	}
}