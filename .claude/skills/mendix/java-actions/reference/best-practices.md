# Java action best practices

Supporting reference for [java-actions](../SKILL.md).

## Part 5: Best Practices

### Naming Conventions

| Element | Convention | Example |
|---------|------------|---------|
| Java Action | `JA_` prefix + PascalCase | `JA_CalculateTax`, `JA_SendEmail` |
| Module | Business domain name | `Finance`, `Integration`, `Utils` |
| Parameters | PascalCase, descriptive | `OrderAmount`, `CustomerEmail` |

### Code Organization (Recommended)

**Keep Java action code minimal** - only handle parameter extraction and delegation. Put the actual implementation in separate classes under `<modulename>.impl`.

**Why?**
- Code between `begin user CODE` and `end user CODE` is preserved, but it's limited space
- Implementation classes are fully under your control (not regenerated)
- Easier to unit test implementation logic separately
- Better code organization and reusability

**Package Structure:**
```
javasource/
├── mymodule/
│   ├── actions/
│   │   └── JA_ProcessOrder.java      # Generated action (minimal code)
│   └── impl/
│       ├── processorder/
│       │   ├── OrderProcessor.java   # Main implementation
│       │   ├── OrderValidator.java   # validation logic
│       │   └── OrderNotifier.java    # notification logic
│       └── shared/
│           └── EmailService.java     # Shared utilities
```

**Example - Java Action (Thin Wrapper):**
```java
// in javasource/mymodule/actions/JA_ProcessOrder.java
@java.lang.Override
public java.lang.Boolean executeAction() throws Exception
{
    // begin user CODE
    // Delegate to implementation class - keep this minimal!
    return new mymodule.impl.processorder.OrderProcessor(getContext())
        .process(this.Order, this.SendNotification);
    // end user CODE
}
```

**Example - Implementation Class (Testable Design):**

The key to testability is separating **pure business logic** from **Mendix API calls**. Use interfaces for data access so you can mock them in tests.

```java
// in javasource/mymodule/impl/processorder/OrderProcessor.java
package mymodule.impl.processorder;

import java.math.BigDecimal;
import java.util.Date;

/**
 * Pure business logic - NO Mendix dependencies!
 * Can be tested with plain JUnit without running Mendix.
 */
public class OrderProcessor {

    public ProcessResult process(OrderData order, boolean sendNotification) {
        // Validate - pure java logic
        if (order.getOrderNumber() == null || order.getOrderNumber().isEmpty()) {
            return ProcessResult.failure("Order number is required");
        }
        if (order.getTotalAmount() == null || order.getTotalAmount().compareTo(BigDecimal.ZERO) <= 0) {
            return ProcessResult.failure("Order amount must be positive");
        }

        // Calculate - pure java logic
        BigDecimal tax = calculateTax(order.getTotalAmount(), order.getTaxRate());
        BigDecimal finalAmount = order.getTotalAmount().add(tax);

        // return result (actual persistence happens in adapter)
        return ProcessResult.success(finalAmount, tax, new date());
    }

    public BigDecimal calculateTax(BigDecimal amount, BigDecimal rate) {
        if (amount == null || rate == null) {
            return BigDecimal.ZERO;
        }
        return amount.multiply(rate).divide(BigDecimal.valueOf(100), 2, java.math.RoundingMode.HALF_UP);
    }
}
```

```java
// in javasource/mymodule/impl/processorder/OrderData.java
package mymodule.impl.processorder;

import java.math.BigDecimal;

/**
 * Plain Java data object - no Mendix dependencies.
 */
public class OrderData {
    private string orderNumber;
    private BigDecimal totalAmount;
    private BigDecimal taxRate;

    // Constructor, getters, setters...
    public OrderData(string orderNumber, BigDecimal totalAmount, BigDecimal taxRate) {
        this.orderNumber = orderNumber;
        this.totalAmount = totalAmount;
        this.taxRate = taxRate;
    }

    public string getOrderNumber() { return orderNumber; }
    public BigDecimal getTotalAmount() { return totalAmount; }
    public BigDecimal getTaxRate() { return taxRate; }
}
```

```java
// in javasource/mymodule/impl/processorder/MendixOrderAdapter.java
package mymodule.impl.processorder;

import com.mendix.systemwideinterfaces.core.IContext;
import com.mendix.systemwideinterfaces.core.IMendixObject;
import com.mendix.core.Core;
import java.math.BigDecimal;

/**
 * Adapter: converts between Mendix objects and pure Java objects.
 * This is the ONLY class that touches Mendix APIs.
 */
public class MendixOrderAdapter {
    private final IContext context;

    public MendixOrderAdapter(IContext context) {
        this.context = context;
    }

    public OrderData toOrderData(IMendixObject mendixOrder) {
        return new OrderData(
            (string) mendixOrder.getValue(context, "OrderNumber"),
            (BigDecimal) mendixOrder.getValue(context, "TotalAmount"),
            (BigDecimal) mendixOrder.getValue(context, "TaxRate")
        );
    }

    public void applyResult(IMendixObject mendixOrder, ProcessResult result) throws Exception {
        mendixOrder.setValue(context, "status", "Processed");
        mendixOrder.setValue(context, "FinalAmount", result.getFinalAmount());
        mendixOrder.setValue(context, "TaxAmount", result.getTaxAmount());
        mendixOrder.setValue(context, "ProcessedDate", result.getProcessedDate());
        Core.commit(context, mendixOrder);
    }
}
```

**Example - Java Action (Wiring Only):**
```java
// in javasource/mymodule/actions/JA_ProcessOrder.java
@java.lang.Override
public java.lang.Boolean executeAction() throws Exception
{
    // begin user CODE
    // Wire up adapter and processor
    MendixOrderAdapter adapter = new MendixOrderAdapter(getContext());
    OrderProcessor processor = new OrderProcessor();

    // Convert Mendix object to plain java object
    OrderData orderData = adapter.toOrderData(this.Order);

    // Process (pure java - no Mendix dependencies)
    ProcessResult result = processor.process(orderData, this.SendNotification);

    if (result.isSuccess()) {
        // apply result back to Mendix object
        adapter.applyResult(this.Order, result);
        return true;
    } else {
        Core.getLogger("MyModule").warn("Order processing failed: " + result.getMessage());
        return false;
    }
    // end user CODE
}
```

**Example - Unit Test (No Mendix Runtime Required):**
```java
// in javasource/mymodule/impl/processorder/OrderProcessorTest.java
package mymodule.impl.processorder;

import org.junit.Test;
import static org.junit.Assert.*;
import java.math.BigDecimal;

public class OrderProcessorTest {

    @Test
    public void testProcessValidOrder() {
        // Arrange - plain java objects, no mocking needed!
        OrderProcessor processor = new OrderProcessor();
        OrderData order = new OrderData("ORD-001", new BigDecimal("100.00"), new BigDecimal("21"));

        // Act
        ProcessResult result = processor.process(order, false);

        // Assert
        assertTrue(result.isSuccess());
        assertEquals(new BigDecimal("21.00"), result.getTaxAmount());
        assertEquals(new BigDecimal("121.00"), result.getFinalAmount());
    }

    @Test
    public void testProcessInvalidOrder_MissingOrderNumber() {
        OrderProcessor processor = new OrderProcessor();
        OrderData order = new OrderData(null, new BigDecimal("100.00"), new BigDecimal("21"));

        ProcessResult result = processor.process(order, false);

        assertFalse(result.isSuccess());
        assertEquals("Order number is required", result.getMessage());
    }

    @Test
    public void testCalculateTax() {
        OrderProcessor processor = new OrderProcessor();

        BigDecimal tax = processor.calculateTax(new BigDecimal("200.00"), new BigDecimal("10"));

        assertEquals(new BigDecimal("20.00"), tax);
    }
}
```

**Run tests with Maven or standalone:**
```bash
# from javasource directory
javac -cp .:junit-4.13.jar mymodule/impl/processorder/*.java
java -cp .:junit-4.13.jar org.junit.runner.JUnitCore mymodule.impl.processorder.OrderProcessorTest
```

**Benefits:**
- **Testable without Mendix** - Run JUnit tests locally or in CI without Mendix runtime
- **Fast feedback** - Unit tests run in milliseconds, not minutes
- **Clear separation** - Business logic is pure Java; Mendix integration is isolated in adapters
- **Reusable** - `OrderProcessor` can be used in other contexts (batch jobs, REST APIs)
- **Maintainable** - Changes to business logic don't require Mendix knowledge

### Error Handling Best Practices

1. **Always wrap in try-catch**:
```java
try {
    // business logic
} catch (Exception e) {
    Core.getLogger("MyModule").error("operation failed", e);
    throw new MendixRuntimeException("user-friendly message: " + e.getMessage());
}
```

2. **Validate inputs early**:
```java
if (this.requiredParam == null) {
    throw new IllegalArgumentException("RequiredParam is required");
}
```

3. **Use appropriate log levels**:
- `trace`: Detailed debugging
- `debug`: Development information
- `info`: Normal operations
- `warn`: Potential issues
- `error`: Recoverable errors
- `critical`: System failures

### Performance Considerations

1. **Batch operations** when possible:
```java
// Instead of committing one by one
list<IMendixObject> toCommit = new ArrayList<>();
for (IMendixObject obj : objects) {
    obj.setValue(context, "status", "Processed");
    toCommit.add(obj);
}
Core.commit(context, toCommit);  // single batch commit
```

2. **Use pagination** for large datasets:
```java
int offset = 0;
int batchSize = 1000;
list<IMendixObject> batch;
do {
    batch = Core.createXPathQuery(xpath).setAmount(batchSize).setOffset(offset).execute(context);
    // Process batch
    offset += batchSize;
} while (batch.size() == batchSize);
```

3. **Cache expensive lookups**:
```java
private static map<string, object> cache = new ConcurrentHashMap<>();
```
