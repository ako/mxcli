# Starlark Lint Rule: Orphaned Elements
#
# This rule checks for elements that are not referenced anywhere in the project.
# Orphaned elements may be:
#   - Dead code that should be removed
#   - Entry points that should be documented
#   - Work in progress that was forgotten
#
# Checks:
#   - Microflows with no callers (except ACT_ which are UI entry points)
#   - Pages with no show_page references (except entry pages)
#   - Entities with no references at all
#
# This rule requires REFRESH CATALOG FULL to populate the refs table.

RULE_ID = "QUAL004"
RULE_NAME = "Orphaned Elements"
DESCRIPTION = "Elements should be referenced somewhere in the project or marked as entry points"
CATEGORY = "quality"
SEVERITY = "info"

# Prefixes for microflows that are expected to be entry points
ENTRY_POINT_PREFIXES = ["ACT_", "SCH_", "WS_", "REST_", "OData_"]

# Page name patterns that are likely entry points
ENTRY_PAGE_PATTERNS = ["Home", "Login", "Index", "Dashboard"]

# Reference kinds that mean "something causes this microflow to run". These are
# catalog RefKind values (mdl/catalog/builder_references.go); a kind missing here
# turns a live document into a false "not called from anywhere" finding.
MICROFLOW_ENTRY_KINDS = ["call", "schedule", "datasource", "action", "calculate"]

# Reference kinds that mean "something opens this page".
PAGE_ENTRY_KINDS = ["show_page", "home_page", "login_page", "menu_item", "action"]

def is_entry_point_microflow(name):
    """Check if a microflow name suggests it's a UI/scheduled entry point."""
    for prefix in ENTRY_POINT_PREFIXES:
        if name.startswith(prefix):
            return True
    return False

def is_entry_point_page(name):
    """Check if a page name suggests it's an entry point."""
    for pattern in ENTRY_PAGE_PATTERNS:
        if pattern in name:
            return True
    return False

def check():
    """
    Check for orphaned elements that have no incoming references.
    """
    violations = []

    # Check microflows
    for mf in microflows():
        # Skip entry point microflows
        if is_entry_point_microflow(mf.name):
            continue

        # Get references to this microflow
        refs = refs_to(mf.qualified_name)

        # Anything that causes the microflow to run counts as a caller, not just
        # a literal "call" edge. A microflow reached only through one of the other
        # kinds was reported as orphaned with the suggestion "Remove if unused":
        #
        #   schedule    a scheduled event runs it (Mendix's cron)
        #   datasource  a page or widget uses it as a data source
        #   action      a widget button calls it
        #   calculate   a calculated attribute computes with it
        #
        # The banking-app report hit the 'datasource' case: DS_CurrentCustomer and
        # DS_MyAccounts are both page data sources and both were flagged.
        has_callers = False
        for ref in refs:
            if ref.ref_kind in MICROFLOW_ENTRY_KINDS:
                has_callers = True
                break

        if not has_callers:
            loc = location(
                module=mf.module_name,
                document_type="Microflow",
                document_name=mf.qualified_name
            )
            v = violation(
                message="Microflow '{}' is not called from anywhere.".format(mf.name),
                location=loc,
                suggestion="Remove if unused, or rename with ACT_/SCH_ prefix if it's an entry point."
            )
            violations.append(v)

    # Check pages
    for page in pages():
        # Skip likely entry point pages
        if is_entry_point_page(page.name):
            continue

        # Get references to this page
        refs = refs_to(page.qualified_name)

        # A page is reachable if anything opens it. Navigation counts: a page that
        # is only a home page, a login page or a menu item is reached by the
        # client, not by a microflow. Counting only 'show_page' reported those as
        # orphaned — masked until now by ENTRY_PAGE_PATTERNS, which happens to
        # cover the pages most likely to be navigation targets.
        is_shown = False
        for ref in refs:
            if ref.ref_kind in PAGE_ENTRY_KINDS:
                is_shown = True
                break

        if not is_shown:
            loc = location(
                module=page.module_name,
                document_type="Page",
                document_name=page.qualified_name
            )
            v = violation(
                message="Page '{}' is not shown from any microflow.".format(page.name),
                location=loc,
                suggestion="Remove if unused, or verify it's configured as a menu item or home page."
            )
            violations.append(v)

    return violations
