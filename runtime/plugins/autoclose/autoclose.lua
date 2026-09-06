VERSION = "1.0.0"

local uutil = import("micro/util")
local utf8 = import("utf8")
local buffer = import("micro/buffer")
local autoclosePairs = {"\"\"", "''", "``", "()", "{}", "[]"}
local autoNewlinePairs = {"()", "{}", "[]"}

function charAt(str, i)
    -- lua indexing is one off from go
    return uutil.RuneAt(str, i-1)
end

function onRune(bp, r)
    for i = 1, #autoclosePairs do
        if r == charAt(autoclosePairs[i], 2) then
            local curLine = bp.Buf:Line(bp.Cursor.Y)

            if charAt(curLine, bp.Cursor.X+1) == charAt(autoclosePairs[i], 2) then
                bp:Backspace()
                bp:CursorRight()
                break
            end

            if bp.Cursor.X > 1 and (uutil.IsWordChar(charAt(curLine, bp.Cursor.X-1)) or charAt(curLine, bp.Cursor.X-1) == charAt(autoclosePairs[i], 1)) then
                break
            end
        end
        if r == charAt(autoclosePairs[i], 1) then
            local curLine = bp.Buf:Line(bp.Cursor.Y)

            if bp.Cursor.X == uutil.CharacterCountInString(curLine) or not uutil.IsWordChar(charAt(curLine, bp.Cursor.X+1)) then
                -- the '-' here is to derefence the pointer to bp.Cursor.Loc which is automatically made
                -- when converting go structs to lua
                -- It needs to be dereferenced because the function expects a non pointer struct
                bp.Buf:Insert(-bp.Cursor.Loc, charAt(autoclosePairs[i], 2))
                bp:CursorLeft()
                break
            end
        end
    end
    return true
end

function preInsertNewline(bp)
    local curLine = bp.Buf:Line(bp.Cursor.Y)
    local curRune = charAt(curLine, bp.Cursor.X)
    local nextRune = charAt(curLine, bp.Cursor.X+1)
    local ws = uutil.GetLeadingWhitespace(curLine)

    for i = 1, #autoNewlinePairs do
        if curRune == charAt(autoNewlinePairs[i], 1) then
            if nextRune == charAt(autoNewlinePairs[i], 2) then
                bp.Buf:Insert(-bp.Cursor.Loc, "\n" .. ws)
                bp:StartOfLine()
                bp:CursorLeft()
                bp:InsertNewline()
                bp:InsertTab()
                return false
            end
        end
    end

    return true
end

function preBackspace(bp)
    for i = 1, #autoclosePairs do
        local curLine = bp.Buf:Line(bp.Cursor.Y)
        if charAt(curLine, bp.Cursor.X+1) == charAt(autoclosePairs[i], 2) and charAt(curLine, bp.Cursor.X) == charAt(autoclosePairs[i], 1) then
            bp:Delete()
        end
    end

    return true
end

-- surroundSelection wraps the cursor's selection in open/close and keeps
-- exactly the original text selected, so typing another opener nests.
-- The two inserts shift the cursor and selection the way typing would,
-- which drags the closer into the selection; hence the explicit reset.
function surroundSelection(bp, open, close)
    local first = -bp.Cursor.CurSelection[1]
    local second = -bp.Cursor.CurSelection[2]
    if first:GreaterThan(second) then
        -- Not "first, second = second, first": the bundled gopher-lua
        -- miscompiles that swap on locals and leaves both equal.
        local earlier = second
        second = first
        first = earlier
    end
    local cursorAtEnd = bp.Cursor.X == second.X and bp.Cursor.Y == second.Y

    bp.Buf:Insert(second, close)
    bp.Buf:Insert(first, open)

    local newFirst = buffer.Loc(first.X + 1, first.Y)
    local newSecond = buffer.Loc(second.X, second.Y)
    if second.Y == first.Y then
        newSecond = buffer.Loc(second.X + 1, second.Y)
    end
    bp.Cursor:SetSelectionStart(newFirst)
    bp.Cursor:SetSelectionEnd(newSecond)
    local cursorLoc = newFirst
    if cursorAtEnd then
        cursorLoc = newSecond
    end
    bp.Cursor.X = cursorLoc.X
    bp.Cursor.Y = cursorLoc.Y
    bp.Cursor:StoreVisualX()
    bp:Relocate()
end

-- Typing an opening character while text is selected wraps the
-- selection in the pair instead of replacing it. DoRuneInsert calls
-- preRune once per cursor, so every cursor with a selection gets its
-- own pair. Returning false skips the normal insert (and onRune).
function preRune(bp, r)
    if not bp.Cursor:HasSelection() then
        return true
    end
    for i = 1, #autoclosePairs do
        if r == charAt(autoclosePairs[i], 1) then
            surroundSelection(bp, charAt(autoclosePairs[i], 1), charAt(autoclosePairs[i], 2))
            return false
        end
    end
    return true
end
