# A REST API, end to end

Supporting reference for [json-structures-and-mappings](../SKILL.md).

One worked example carried the whole way: a JSON structure per payload shape, an
import mapping for a single object and for a list, an export mapping in both
directions, and the microflow that ties them together. Copy and adapt it rather
than assembling the parts from the syntax tables.

## Realistic Example: Countries REST API

A complete example consuming a Countries REST API, importing the response, and
exporting country data back to JSON.

### Step 1: JSON Structures

```sql
-- Single country (flat object)
create json structure Integration.JSON_Country
  snippet '{"name": "Netherlands", "officialName": "Kingdom of the Netherlands", "capital": "Amsterdam", "region": "Europe", "population": 18100436, "flagUrl": "https://flagcdn.com/w320/nl.png"}';

-- List of countries (array of objects)
create json structure Integration.JSON_CountryList
  snippet '[{"name": "Netherlands", "capital": "Amsterdam", "region": "Europe", "population": 18100436}]';
```

### Step 2: Import — Single Country

```sql
create non-persistent entity Integration.Country (
  Name: string,
  OfficialName: string,
  Capital: string,
  Region: string,
  Population: integer,
  FlagUrl: string
);
/

create import mapping Integration.IMM_Country
  with json structure Integration.JSON_Country
{
  create Integration.Country {
    Name = name,
    OfficialName = officialName,
    Capital = capital,
    Region = region,
    Population = population,
    FlagUrl = flagUrl
  }
};
```

### Step 3: Import — List of Countries

For a list response, the import mapping maps the array item directly (no container):

```sql
create non-persistent entity Integration.CountryListItem (
  Name: string,
  Capital: string,
  Region: string,
  Population: integer
);
/

create import mapping Integration.IMM_CountryList
  with json structure Integration.JSON_CountryList
{
  create Integration.CountryListItem {
    Name = name,
    Capital = capital,
    Region = region,
    Population = population
  }
};
```

### Step 4: Export — Serialize Country to JSON

For the flat country, the same entity works for both import and export:

```sql
create export mapping Integration.EMM_Country
  with json structure Integration.JSON_Country
{
  Integration.Country {
    name = Name,
    officialName = OfficialName,
    capital = Capital,
    region = Region,
    population = Population,
    flagUrl = FlagUrl
  }
};
```

### Step 5: Export — List of Countries

For exporting a list, the export domain model needs a root container + item entities:

```sql
-- Container entity wrapping the array
create non-persistent entity Integration.ExCountryList;
/

-- Item entity for each country in the array
create non-persistent entity Integration.ExCountryItem (
  Name: string,
  Capital: string,
  Region: string,
  Population: integer
);
/

create association Integration.ExCountryItem_ExCountryList
  from Integration.ExCountryItem
  to Integration.ExCountryList;
/

create export mapping Integration.EMM_CountryList
  with json structure Integration.JSON_CountryList
{
  Integration.ExCountryList {
    Integration.ExCountryItem_ExCountryList/Integration.ExCountryItem as Root {
      name = Name,
      capital = Capital,
      region = Region,
      population = Population
    }
  }
};
```

### Step 6: Microflow — Fetch, Import, Process, Export

```sql
create microflow Integration.GetCountryInfo ()
returns string as $json
begin
  -- Fetch country data from REST API
  $response = rest call get 'https://restcountries.com/v3.1/name/netherlands'
    header Accept = 'application/json'
    timeout 30
    returns string
    on error continue;

  -- Import JSON into entity
  $Country = import from mapping Integration.IMM_Country($response);

  -- Export back to our own JSON format
  $json = export to mapping Integration.EMM_Country($Country);
  log info node 'Integration' 'Country: ' + $json;

  return $json;
end;
/
```

---
