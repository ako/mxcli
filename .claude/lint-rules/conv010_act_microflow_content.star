# CONV010: ACT_ Microflow Content Restriction
#
# Microflows prefixed with ACT_ are page action microflows. They should only
# contain UI-related activities:
#   - ShowPageAction (show page)
#   - ClosePageAction (close page)
#   - ShowHomePageAction (show home page)
#   - ShowMessageAction (show message)
#   - DownloadFileAction (download file)
#   - MicroflowCallAction (call sub-microflow for logic delegation)
#
# Business logic should be delegated to SUB_ microflows.
# Requires FULL catalog (REFRESH CATALOG FULL).
#
# NOTE ON NAMES: the catalog labels an action with its *SDK* type name, derived
# from the parsed action's Go type (catalog.getMicroflowActionType). That is not
# always the name Mendix uses in BSON: ShowPageAction is stored as
# "Microflows$ShowFormAction", ClosePageAction as "CloseFormAction", and so on
# (see the storage-name table in CLAUDE.md). This rule matches what the linter
# sees, so it must use the SDK names — it previously used the storage names and
# therefore matched nothing, flagging every ACT_ microflow that showed a page,
# closed one, or called a sub-microflow. Both spellings are listed so the rule
# keeps working if the catalog's vocabulary is ever changed to the storage names.

RULE_ID = "CONV010"
RULE_NAME = "ACTMicroflowContent"
DESCRIPTION = "ACT_ microflows should only contain UI actions and sub-microflow calls"
CATEGORY = "architecture"
SEVERITY = "warning"

# Allowed action types in ACT_ microflows
ALLOWED_ACTIONS = (
    # SDK names — what the catalog actually reports.
    "ShowPageAction",
    "ClosePageAction",
    "ShowHomePageAction",
    "ShowMessageAction",
    "DownloadFileAction",
    "MicroflowCallAction",
    # Storage names — belt and braces; see the note above.
    "ShowFormAction",
    "CloseFormAction",
    "ShowHomeFormAction",
)

# Allowed activity types (non-action activities)
ALLOWED_ACTIVITY_TYPES = (
    "SubMicroflow",
    "MicroflowCallAction",
    "StartEvent",
    "EndEvent",
    "ExclusiveSplit",
    "Annotation",
)

def check():
    violations = []

    for mf in microflows():
        if not mf.name.startswith("ACT_"):
            continue

        for act in activities_for(mf.qualified_name):
            # Skip allowed activity types
            if act.activity_type in ALLOWED_ACTIVITY_TYPES:
                continue

            # For ActionActivity, check the action type
            if act.activity_type == "ActionActivity":
                if act.action_type in ALLOWED_ACTIONS:
                    continue

                violations.append(violation(
                    message="ACT_ microflow '{}' contains '{}' action. Delegate business logic to a SUB_ microflow.".format(
                        mf.name, act.action_type
                    ),
                    location=location(
                        module=mf.module_name,
                        document_type="Microflow",
                        document_name=mf.qualified_name,
                    ),
                    suggestion="Move the '{}' action to a SUB_ microflow and call it from '{}'".format(
                        act.action_type, mf.name
                    ),
                ))
            elif act.activity_type not in ALLOWED_ACTIVITY_TYPES:
                # Any other non-allowed activity type
                violations.append(violation(
                    message="ACT_ microflow '{}' contains '{}' activity. Delegate to a SUB_ microflow.".format(
                        mf.name, act.activity_type
                    ),
                    location=location(
                        module=mf.module_name,
                        document_type="Microflow",
                        document_name=mf.qualified_name,
                    ),
                    suggestion="Move the '{}' to a SUB_ microflow called from '{}'".format(
                        act.activity_type, mf.name
                    ),
                ))

    return violations
