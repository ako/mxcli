/**
 * MDL Security Grammar — security statements (module roles, user roles,
 * grants, revokes, project security, demo users).
 */
parser grammar MDLSecurity;

options { tokenVocab = MDLLexer; }

// =============================================================================
// SECURITY STATEMENTS
// =============================================================================

// OR MODIFY makes a security script re-runnable. Without it, re-executing the
// script that sets up roles fails on the first role that already exists, so
// role creation had to live in its own run-once file.
createModuleRoleStatement
    : CREATE (OR MODIFY)? MODULE ROLE qualifiedName (DESCRIPTION STRING_LITERAL)?
    ;

dropModuleRoleStatement
    : DROP MODULE ROLE qualifiedName
    ;

createUserRoleStatement
    : USER ROLE identifierOrKeyword
      LPAREN moduleRoleList RPAREN
      (MANAGE ALL ROLES)?
    ;

alterUserRoleStatement
    : ALTER USER ROLE identifierOrKeyword ADD MODULE ROLES LPAREN moduleRoleList RPAREN
    | ALTER USER ROLE identifierOrKeyword REMOVE MODULE ROLES LPAREN moduleRoleList RPAREN
    ;

dropUserRoleStatement
    : DROP USER ROLE (identifierOrKeyword | STRING_LITERAL)
    ;

grantEntityAccessStatement
    : GRANT moduleRoleList ON qualifiedName
      LPAREN entityAccessRightList RPAREN
      (WHERE STRING_LITERAL)?
    ;

revokeEntityAccessStatement
    : REVOKE moduleRoleList ON qualifiedName
      (LPAREN entityAccessRightList RPAREN)?
    ;

grantMicroflowAccessStatement
    : GRANT EXECUTE ON MICROFLOW qualifiedName TO moduleRoleList
    ;

revokeMicroflowAccessStatement
    : REVOKE EXECUTE ON MICROFLOW qualifiedName FROM moduleRoleList
    ;

grantNanoflowAccessStatement
    : GRANT EXECUTE ON NANOFLOW qualifiedName TO moduleRoleList
    ;

revokeNanoflowAccessStatement
    : REVOKE EXECUTE ON NANOFLOW qualifiedName FROM moduleRoleList
    ;

grantPageAccessStatement
    : GRANT VIEW ON PAGE qualifiedName TO moduleRoleList
    ;

revokePageAccessStatement
    : REVOKE VIEW ON PAGE qualifiedName FROM moduleRoleList
    ;

grantWorkflowAccessStatement
    : GRANT EXECUTE ON WORKFLOW qualifiedName TO moduleRoleList
    ;

revokeWorkflowAccessStatement
    : REVOKE EXECUTE ON WORKFLOW qualifiedName FROM moduleRoleList
    ;

grantODataServiceAccessStatement
    : GRANT ACCESS ON ODATA SERVICE qualifiedName TO moduleRoleList
    ;

revokeODataServiceAccessStatement
    : REVOKE ACCESS ON ODATA SERVICE qualifiedName FROM moduleRoleList
    ;

grantPublishedRestServiceAccessStatement
    : GRANT ACCESS ON PUBLISHED REST SERVICE qualifiedName TO moduleRoleList
    ;

revokePublishedRestServiceAccessStatement
    : REVOKE ACCESS ON PUBLISHED REST SERVICE qualifiedName FROM moduleRoleList
    ;

alterProjectSecurityStatement
    : ALTER PROJECT SECURITY LEVEL (PRODUCTION | PROTOTYPE | OFF)
    | ALTER PROJECT SECURITY DEMO USERS (ON | OFF)
    // ROLE is optional here but effectively required by Mendix: mxbuild raises
    // CE0133 when guest access is on with no role. It is optional so that
    // re-enabling a project that already stores one does not force a retype;
    // the executor refuses ON when neither source supplies a role.
    | ALTER PROJECT SECURITY GUEST ACCESS ON (ROLE identifierOrKeyword)?
    | ALTER PROJECT SECURITY GUEST ACCESS OFF
    ;

createDemoUserStatement
    : DEMO USER STRING_LITERAL PASSWORD STRING_LITERAL (ENTITY qualifiedName)?
      LPAREN identifierOrKeyword (COMMA identifierOrKeyword)* RPAREN
    ;

dropDemoUserStatement
    : DROP DEMO USER STRING_LITERAL
    ;

updateSecurityStatement
    : UPDATE SECURITY (IN qualifiedName)?
    ;

moduleRoleList
    : qualifiedName (COMMA qualifiedName)*
    ;

entityAccessRightList
    : entityAccessRight (COMMA entityAccessRight)*
    ;

entityAccessRight
    : CREATE
    | DELETE
    | READ STAR
    | READ LPAREN entityMemberName (COMMA entityMemberName)* RPAREN
    | WRITE STAR
    | WRITE LPAREN entityMemberName (COMMA entityMemberName)* RPAREN
    ;

// Member (attribute / association) name in a READ/WRITE list. Accepts a quoted
// identifier so members whose name is a reserved word can be escaped, e.g.
// READ ("Order", Status).
entityMemberName
    : IDENTIFIER
    | QUOTED_IDENTIFIER
    ;
