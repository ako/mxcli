# OQL patterns and a worked example

Supporting reference for [write-oql-queries](../SKILL.md).

## Common OQL Patterns

### Pattern 1: Date-based Aggregation
```sql
create view entity Finance.MonthlySummary (
  Year: integer,
  Month: integer,
  TotalAmount: decimal,
  TransactionCount: integer
) as (
  select
    datepart(YEAR, t.Date) as Year,
    datepart(MONTH, t.Date) as Month,
    sum(t.Amount) as TotalAmount,
    count(t.ID) as TransactionCount
  from Finance.Transaction as t
  where t.Status != 'VOIDED'
  GROUP by datepart(YEAR, t.Date), datepart(MONTH, t.Date)
);
```

### Pattern 2: Conditional Aggregation
```sql
create view entity Finance.CategorySummary (
  Category: string(200),
  Income: decimal,
  Expense: decimal,
  Net: decimal
) as (
  select
    c.Name as Category,
    sum(case when t.Type = 'INCOME' then t.Amount else 0 end) as Income,
    sum(case when t.Type = 'EXPENSE' then t.Amount else 0 end) as Expense,
    sum(case when t.Type = 'INCOME' then t.Amount
             when t.Type = 'EXPENSE' then -t.Amount else 0 end) as Net
  from Finance.Transaction as t
  inner join Finance.Transaction_Category/Finance.Category as c
  GROUP by c.Name
);
```

### Pattern 3: Association Navigation
```sql
create view entity Shop.OrderDetails (
  OrderId: long,
  CustomerName: string(400),
  TotalItems: integer,
  TotalPrice: decimal
) as (
  select
    o.OrderId as OrderId,
    c.FirstName + ' ' + c.LastName as CustomerName,
    count(ol.OrderLineId) as TotalItems,
    o.TotalPrice as TotalPrice
  from Shop.CustomerOrder as o
  inner join Shop.Order_Customer/Shop.Customer as c
  left join Shop.OrderLine_Order/Shop.OrderLine as ol
  GROUP by o.OrderId, o.TotalPrice, c.FirstName, c.LastName
);
```

### Pattern 4: Calculations with Division
```sql
create view entity Finance.BudgetVariance (
  Category: string(200),
  Budget: decimal,
  Actual: decimal,
  Variance: decimal,
  VariancePercent: decimal
) as (
  select
    c.Name as Category,
    bl.PlannedAmount as Budget,
    bl.ActualAmount as Actual,
    bl.ActualAmount - bl.PlannedAmount as Variance,
    (bl.ActualAmount - bl.PlannedAmount) * 100.0 : bl.PlannedAmount as VariancePercent
  from Finance.BudgetLine as bl
  inner join Finance.BudgetLine_Category/Finance.Category as c
  where bl.PlannedAmount > 0
);
```

### Pattern 5: IN Expression with Value List
```sql
create view entity Shop.HighPriorityTasks (
  TaskId: integer,
  TaskTitle: string(200),
  Priority: string(50)
) as (
  select
    t.TaskId as TaskId,
    t.TaskTitle as TaskTitle,
    t.TaskPriority as Priority
  from Shop.Task as t
  where t.TaskPriority in ('HIGH', 'CRITICAL')
);
```

### Pattern 6: IN Expression with Subquery
```sql
create view entity Shop.CustomersWithOrders (
  CustomerId: integer,
  CustomerName: string(200)
) as (
  select
    c.CustomerId as CustomerId,
    c.Name as CustomerName
  from Shop.Customer as c
  where c.CustomerId in (
    select distinct o.CustomerId
    from Shop.Order as o
    where o.Status = 'COMPLETED'
  )
);
```

### Pattern 7: Scalar Subquery in SELECT
```sql
create view entity Shop.ProductsAboveAverage (
  ProductId: integer,
  Name: string(200),
  Price: decimal,
  PriceDifferenceFromAvg: decimal
) as (
  select
    p.ProductId as ProductId,
    p.Name as Name,
    p.Price as Price,
    p.Price - (select avg(p2.Price) from Shop.Product as p2) as PriceDifferenceFromAvg
  from Shop.Product as p
  where p.Price > (select avg(p3.Price) from Shop.Product as p3)
);
```

### Pattern 8: Correlated Subquery
```sql
create view entity Shop.OrdersWithCustomerStats (
  OrderId: integer,
  OrderNumber: string(50),
  CustomerTotalOrders: integer,
  CustomerTotalSpend: decimal
) as (
  select
    o.OrderId as OrderId,
    o.OrderNumber as OrderNumber,
    (select count(o2.OrderId) from Shop.Order as o2 where o2.CustomerId = o.CustomerId) as CustomerTotalOrders,
    (select sum(o3.TotalAmount) from Shop.Order as o3 where o3.CustomerId = o.CustomerId) as CustomerTotalSpend
  from Shop.Order as o
);
```

### Pattern 9: Correlated Subquery via Association
```sql
-- Get the latest price for each product using association traversal
create view entity Shop.ProductCurrentPrice (
  ProductId: string(50),
  Name: string(200),
  PriceInEuro: decimal,
  IsActive: boolean
) as (
  select
    p.ProductId as ProductId,
    p.Name as Name,
    (select pr.PriceInEuro
     from Shop.Price as pr
     where pr.StartDate <= '[%BeginOfTomorrow%]'
     and pr/Shop.Price_Product = p.ID
     ORDER by pr.StartDate desc
     limit 1) as PriceInEuro,
    p.IsActive as IsActive
  from Shop.Product as p
  where p.IsActive
);
```

**Key points:**
- Use `pr/Shop.Price_Product = p.ID` (association path with `.ID`)
- Never use bare alias: `pr/Shop.Price_Product = p` will fail
- ORDER BY and LIMIT are valid inside correlated subqueries; at the view level, ORDER BY is allowed only when paired with LIMIT (MDL030)

### Pattern 10: JOIN with ON Clause (Non-Association)
```sql
-- When joining on arbitrary conditions (not Mendix associations)
create view entity Shop.ProductComparison (
  ProductId: integer,
  ProductName: string(200),
  CompetitorPrice: decimal
) as (
  select
    p.ProductId as ProductId,
    p.Name as ProductName,
    cp.Price as CompetitorPrice
  from Shop.Product as p
  left join Shop.CompetitorProduct as cp on p.ProductCode = cp.ProductCode
  where cp.CompetitorName = 'ACME'
);
```
## Complete Example

### User Request
"Create a VIEW entity showing monthly revenue with order statistics"

### Response
```sql
/**
 * Monthly revenue summary with order statistics
 *
 * Time-series view of revenue and order metrics
 * aggregated by month and year.
 *
 * @since 1.0.0
 * @see Shop.CustomerOrder
 */
@position(1400, 450)
create view entity Shop.MonthlyRevenue (
  Year: integer,
  Month: integer,
  TotalOrders: integer,
  TotalRevenue: decimal,
  AverageOrderValue: decimal
) as (
  select
    datepart(YEAR, o.OrderDate) as Year,
    datepart(MONTH, o.OrderDate) as Month,
    count(o.OrderId) as TotalOrders,
    sum(o.TotalPrice) as TotalRevenue,
    avg(o.TotalPrice) as AverageOrderValue
  from Shop.CustomerOrder as o
  GROUP by datepart(YEAR, o.OrderDate), datepart(MONTH, o.OrderDate)
);
```

### Why This Works
1. ✅ All columns have explicit AS aliases
2. ✅ Lowercase aggregates: `sum()`, `avg()`, `count()`
3. ✅ Proper COUNT: `count(o.OrderId)` not `count(*)`
4. ✅ Comma syntax for DATEPART: `datepart(YEAR, o.OrderDate)`
5. ✅ GROUP BY matches SELECT non-aggregated expressions
6. ✅ ORDER BY omitted (UI sorts) — or, for a top-N view, paired with a LIMIT (MDL030)
