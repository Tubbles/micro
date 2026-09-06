VERSION = "1.0.0"

local micro = import("micro")
local config = import("micro/config")
local shell = import("micro/shell")
local util = import("micro/util")
local ioutil = import("io/ioutil")

-- kroken takes 1-based positions with an inclusive end, while micro's
-- selection end is exclusive. An end at column 0 means the selection
-- runs through the end of the previous line, newline included.
local function positionArguments(first, second)
    local startArgument = (first.Y + 1) .. ":" .. (first.X + 1)
    if second.X == 0 then
        return startArgument, tostring(second.Y)
    end
    return startArgument, (second.Y + 1) .. ":" .. second.X
end

-- Returns the first line of text that starts with prefix, else fallback.
local function lineWithPrefix(text, prefix, fallback)
    for line in text:gmatch("[^\n]+") do
        if line:sub(1, #prefix) == prefix then
            return line
        end
    end
    return fallback
end

-- Runs on the main thread once the kroken process has exited. Micro's
-- job API hands the exit callback stdout and stderr mixed together, so
-- the two streams are collected separately by the stream callbacks and
-- the exit callback's own argument is ignored.
local function onExit(run)
    os.remove(run.selectionFile)
    run.buf:RemoveAnchor(run.anchor)

    local processState = run.job.ProcessState
    if processState == nil then
        micro.InfoBar():Error("kroken: could not start kroken, is it on your PATH?")
        return
    end
    local exitCode = processState:ExitCode()
    if exitCode ~= 0 then
        micro.InfoBar():Error(lineWithPrefix(run.stderr, "kroken:", "kroken: exited with status " .. exitCode))
        return
    end
    if util.String(run.anchor:Text()) ~= run.original then
        local logLine = lineWithPrefix(run.stderr, "kroken: log:", "")
        micro.InfoBar():Error("kroken: selection changed while running, result not applied. " .. logLine)
        return
    end
    run.buf:Replace(run.anchor:Start(), run.anchor:End(), run.stdout)
    micro.InfoBar():Message(lineWithPrefix(run.stderr, "kroken: done", "kroken: replaced selection"))
end

function complete(bp, arguments)
    local buf = bp.Buf
    if buf.Path == "" then
        micro.InfoBar():Error("kroken: save the buffer to a file first")
        return
    end
    local cursor = buf:GetActiveCursor()
    if not cursor:HasSelection() then
        micro.InfoBar():Error("kroken: select something first")
        return
    end
    local first = -cursor.CurSelection[1]
    local second = -cursor.CurSelection[2]
    if first:GreaterThan(second) then
        first, second = second, first
    end
    local original = util.String(cursor:GetSelection())

    -- The selection travels in a file: micro can write to a job's stdin
    -- but never closes it, and kroken reads stdin until end of file.
    local selectionFile = os.tmpname()
    local err = ioutil.WriteFile(selectionFile, original, 384)
    if err ~= nil then
        micro.InfoBar():Error("kroken: " .. err:Error())
        return
    end

    local startArgument, endArgument = positionArguments(first, second)
    local commandArguments = {
        "complete",
        "--file", buf.AbsPath,
        "--start", startArgument,
        "--end", endArgument,
        "--selection-file", selectionFile,
    }
    for index = 1, #arguments do
        table.insert(commandArguments, arguments[index])
    end

    -- Every run owns its state, so any number of runs can overlap.
    local run = {
        buf = buf,
        anchor = buf:AddAnchor(first, second),
        original = original,
        selectionFile = selectionFile,
        stdout = "",
        stderr = "",
    }
    run.job = shell.JobSpawn("kroken", commandArguments,
        function(chunk) run.stdout = run.stdout .. chunk end,
        function(chunk) run.stderr = run.stderr .. chunk end,
        function() onExit(run) end)
    micro.InfoBar():Message("kroken: running")
end

function init()
    config.MakeCommand("kroken", complete, config.NoComplete)
    config.TryBindKey("Alt-Enter", "command:kroken", false)
    config.AddRuntimeFile("kroken", config.RTHelp, "help/kroken.md")
end
