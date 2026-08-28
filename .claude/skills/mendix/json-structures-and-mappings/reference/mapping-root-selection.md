# Starting a mapping below the payload root

Supporting reference for [json-structures-and-mappings](../SKILL.md).

A mapping does not have to start at the top of the JSON. `root a/b/c` on the
source clause selects the element it starts at — the same choice Studio Pro
offers when you pick a node deeper in the payload. Useful when the interesting
object is buried under an envelope you do not want entities for.

The path is written in **member names**, and it may pass **through arrays**: the
mapping is then rooted at the array's item. (A value reference cannot do that —
many items cannot collapse into one value, and mxbuild reports CE0256.)

```sql
create json structure RootDemo.JSON_Completion
  snippet $${
    "requestId": "r-1",
    "response": {
      "model": "gpt-x",
      "choices": [
        {
          "index": 0,
          "message": {
            "role": "assistant",
            "content": "hello",
            "citations": [ { "title": "t", "url": "u" } ]
          }
        }
      ]
    }
  }$$;
```

**Through an array, to an object several levels down.** Everything inside the
statement is relative to the selected root, associations included:

```sql
create import mapping RootDemo.IMM_Answer
  with json structure RootDemo.JSON_Completion root response/choices/message
{
  create RootDemo.Answer {
    Role = role,
    Content = content,
    create RootDemo.Citation_Answer/RootDemo.Citation = citations {
      Title = title,
      Url = url
    }
  }
};
```

stored as one root element at `(Object)|response|choices|(Object)|message`, with
`citations` nesting under it as usual.

**Landing on an array.** A root that ends on an array roots the mapping at its
**item**, so `Index` below is a member of one choice, not of the list:

```sql
create import mapping RootDemo.IMM_Choice
  with json structure RootDemo.JSON_Completion root response/choices
{
  create RootDemo.Choice {
    Index = index
  }
};
```

stored at `(Object)|response|choices|(Object)`.

**Export takes the same clause**, and produces the envelope down to the selected
element:

```sql
create export mapping RootDemo.EXM_Answer
  with json structure RootDemo.JSON_Completion root response/choices/message
{
  RootDemo.Answer {
    role = Role,
    content = Content,
    RootDemo.Citation_Answer/RootDemo.Citation as citations {
      title = Title,
      url = Url
    }
  }
};
```

### Notes

- **Omit the clause** and the mapping starts at the structure's own root — an
  array-rooted structure included, which needs no syntax of its own.
- **DESCRIBE emits the clause** for any mapping stored below the root, including
  ones authored in Studio Pro, so `describe` → `exec` round-trips. Re-running a
  described mapping reports `Unchanged import mapping …`: the rebuild is
  semantically equal and the write is elided.
- **A path that does not resolve is refused**, and the error names what would
  have worked:

  ```
  Error: import mapping RootDemo.IMM_Bad: root "response/choise": "choise" is not
  a member of the schema at (Object)|response; available: model (or Model),
  choices (or Choices)
  ```

- The selected root does **not** have to be an object. Any element the structure
  contains can be picked; a value root would leave nothing to map, so in practice
  it is an object or an array.
