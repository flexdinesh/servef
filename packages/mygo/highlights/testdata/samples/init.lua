-- Lua sample
local M = {}

--[[ block
comment ]]
function M.greet(name, ...)
  local n = select("#", ...)
  if name == nil then return nil end
  return ("hi %s %d"):format(name, n) .. [[long]] .. 0x10
end

return M
