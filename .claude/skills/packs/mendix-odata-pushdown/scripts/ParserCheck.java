// A self-check for ODataQueryParser that needs no Mendix runtime.
//
// The parser takes no Mendix types in its signature — strings in, strings out —
// which is the whole reason it is a separate class from QueryObject. That makes
// a dialect or grammar regression checkable in about a second, where every other
// test of this component needs an app, a database and a request.
//
// Run it from the pack directory after substitution:
//
//   mkdir -p /tmp/pc && sed -e 's/{{MODULE_PATH}}/odatapushdown/g' \
//        -e 's/{{MODULE}}/ODataPushdown/g' java/ODataQueryParser.java \
//        > /tmp/pc/ODataQueryParser.java
//   javac -d /tmp/pc /tmp/pc/ODataQueryParser.java scripts/ParserCheck.java
//   java -cp /tmp/pc ParserCheck
//
// Exits non-zero on the first failure, and prints what it expected.
import odatapushdown.ODataQueryParser;

public class ParserCheck {

    private static final String COLS =
            "period:t.period:text,category:t.category:text,total:t.total:number";
    private static int failures = 0;

    public static void main(String[] args) {
        // $select — the projection. Not a correctness fix (Mendix projects the
        // response itself); this narrows what the SOURCE reads.
        eq("no $select leaves the caller's own list alone", sel("/Rows"), "");
        eq("a single column, plus the key",
                sel("/Rows?$select=category"), " t.category, t.period");
        eq("order follows the request, key appended",
                sel("/Rows?$select=category,total"), " t.category, t.total, t.period");
        eq("the key is not duplicated when already asked for",
                sel("/Rows?$select=period"), " t.period");
        eq("$select=* is everything", sel("/Rows?$select=*"), "");
        eq("exposed names come back for binding callers",
                parse("/Rows?$select=total").selectedColumns, "total,period");

        // An unknown column is REJECTED, not skipped: omitting a field the
        // client asked for answers with a null where data was expected.
        yes("an unknown $select column is rejected", parse("/Rows?$select=nope").rejected);
        no("a known one is not", parse("/Rows?$select=total").rejected);

        // The options that were already here, so this is a check of the
        // component and not only of the newest part of it.
        yes("$filter reaches SQL", parse("/Rows?$filter=category eq 'Rent'")
                .filterSql.contains("t.category"));
        yes("a numeric column is compared unquoted",
                parse("/Rows?$filter=total eq 1500").filterSql.contains("1500")
             && !parse("/Rows?$filter=total eq 1500").filterSql.contains("'1500'"));
        yes("a quoted literal on a numeric column is unquoted too — a combo box "
                + "sends 'x' where a grid header sends x",
                !parse("/Rows?$filter=total eq '1500'").filterSql.contains("'1500'"));
        eq("$orderby is read as an exposed name",
                parse("/Rows?$orderby=total desc").sortColumn1, "total");
        eq("...with its direction", parse("/Rows?$orderby=total desc").sortDirection1, "D");
        yes("$top clamps to maxTop", parse("/Rows?$top=99999").top <= 500);
        yes("$count is seen", parse("/Rows?$count=true").wantsCount);
        yes("an unreadable $filter is rejected rather than dropped",
                parse("/Rows?$filter=category eq").rejected);

        if (failures > 0) {
            System.out.println(failures + " check(s) failed");
            System.exit(1);
        }
        System.out.println("ParserCheck: all checks passed");
    }

    private static ODataQueryParser.Result parse(String uri) {
        return ODataQueryParser.parse(uri, COLS, "duckdb", 500, 100, "", "period");
    }

    private static String sel(String uri) {
        return parse(uri).selectSql;
    }

    private static void eq(String what, String got, String want) {
        if (!want.equals(got)) {
            System.out.println("FAIL " + what + "\n  got  '" + got + "'\n  want '" + want + "'");
            failures++;
        }
    }

    private static void yes(String what, boolean got) {
        if (!got) {
            System.out.println("FAIL " + what);
            failures++;
        }
    }

    private static void no(String what, boolean got) {
        if (got) {
            System.out.println("FAIL " + what + " (expected false)");
            failures++;
        }
    }
}
