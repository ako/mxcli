/**
 * MDL Agent Grammar — agent editor: models, knowledge bases,
 * consumed MCP services, agents.
 */
parser grammar MDLAgent;

options { tokenVocab = MDLLexer; }

// =============================================================================
// AGENT-EDITOR MODEL CREATION
// =============================================================================
// CREATE MODEL Module.Name (
//   Provider: MxCloudGenAI,
//   Key: Module.SomeConstant
//   [, DisplayName: '...', KeyName: '...', etc. — Portal-populated metadata]
// );
createModelStatement
    : MODEL qualifiedName
      LPAREN modelProperty (COMMA modelProperty)* RPAREN
    ;

modelProperty
    : identifierOrKeyword COLON identifierOrKeyword       // Provider: MxCloudGenAI
    | identifierOrKeyword COLON qualifiedName             // Key: Module.Constant
    | identifierOrKeyword COLON STRING_LITERAL            // DisplayName: 'GPT-4 Turbo' etc.
    | identifierOrKeyword COLON NUMBER_LITERAL            // ConnectionTimeoutSeconds: 30
    | identifierOrKeyword COLON booleanLiteral            // Enabled: true
    | identifierOrKeyword COLON DOLLAR_STRING              // SystemPrompt: $$multi-line...$$
    | identifierOrKeyword COLON LPAREN variableDefList RPAREN  // Variables: ("Key": EntityAttribute, ...)
    ;

variableDefList
    : variableDef (COMMA variableDef)*
    ;

variableDef
    : (STRING_LITERAL | QUOTED_IDENTIFIER) COLON identifierOrKeyword  // "Key": EntityAttribute
    ;

// =============================================================================
// AGENT-EDITOR CONSUMED MCP SERVICE CREATION
// =============================================================================
// CREATE CONSUMED MCP SERVICE Module.Name (
//   ProtocolVersion: v2025_03_26,
//   Version: '0.0.1',
//   ConnectionTimeoutSeconds: 30,
//   Documentation: '...'
// );
createConsumedMCPServiceStatement
    : CONSUMED MCP SERVICE qualifiedName
      LPAREN modelProperty (COMMA modelProperty)* RPAREN
    ;

// =============================================================================
// AGENT-EDITOR KNOWLEDGE BASE CREATION
// =============================================================================
// CREATE KNOWLEDGE BASE Module.Name (
//   Provider: MxCloudGenAI,
//   Key: Module.SomeConstant
// );
createKnowledgeBaseStatement
    : KNOWLEDGE BASE qualifiedName
      LPAREN modelProperty (COMMA modelProperty)* RPAREN
    ;

// =============================================================================
// AGENT-EDITOR AGENT CREATION
// =============================================================================
// CREATE AGENT Module.Name (
//   UsageType: Task,
//   Model: Module.MyModel,
//   SystemPrompt: '...',
//   ...
// )
// [ { TOOL ... | MCP SERVICE ... | KNOWLEDGE BASE ... } ]
// ;
createAgentStatement
    : AGENT qualifiedName
      LPAREN modelProperty (COMMA modelProperty)* RPAREN
      agentBody?
    ;

agentBody
    : LBRACE agentBodyBlock* RBRACE
    ;

agentBodyBlock
    : MCP SERVICE qualifiedName LBRACE modelProperty (COMMA modelProperty)* RBRACE       // MCP SERVICE Mod.Name { ... }
    | KNOWLEDGE BASE identifierOrKeyword LBRACE modelProperty (COMMA modelProperty)* RBRACE // KNOWLEDGE BASE MyKB { ... }
    | TOOL identifierOrKeyword LBRACE modelProperty (COMMA modelProperty)* RBRACE        // TOOL ToolName { ... }
    ;

// =============================================================================
// ALTER actions for agent-editor documents
// =============================================================================
// Used by ALTER MODEL / ALTER KNOWLEDGE BASE / ALTER CONSUMED MCP SERVICE
// (SET-only — these document types have no collections), and by the SET
// clause of ALTER AGENT.
agentEditorAlterAssignment
    : identifierOrKeyword EQUALS agentEditorAlterValue
    ;

agentEditorAlterValue
    : STRING_LITERAL
    | NUMBER_LITERAL
    | DOLLAR_STRING
    | booleanLiteral
    | qualifiedName
    | identifierOrKeyword
    ;

// ALTER AGENT actions: a mix of SET (scalar properties) and ADD/DROP for
// the agent's three collections (tools, MCP services, knowledge bases).
//
// @example Combine SET with collection mutations:
// ```mdl
// ALTER AGENT MyModule.Helper
//   SET SystemPrompt = 'New prompt', Temperature = 0.5
//   ADD TOOL DoSomething { Description: '...', Enabled: true }
//   ADD MCP SERVICE MyModule.Weather { Description: '...', Enabled: true }
//   DROP KNOWLEDGE BASE OldKB
// ;
// ```
alterAgentAction
    : SET agentEditorAlterAssignment (COMMA agentEditorAlterAssignment)*
    | ADD agentBodyBlock
    | DROP TOOL identifierOrKeyword
    | DROP MCP SERVICE qualifiedName
    | DROP KNOWLEDGE BASE identifierOrKeyword
    ;
