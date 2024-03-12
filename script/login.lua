local headers = { }
wrk.method = "POST"
wrk.path = "/login"
headers['Content-Type'] = "application/x-www-form-urlencoded"
math.randomseed(42)
request = function()
    id = tostring(math.random(1,200))
    body = "username=ray"..id.."&password=pwd"
   return wrk.format(nil, wrk.path, headers, body)
end

-- wrk -t12 -c1000 -d30s http://localhost/login -s login.lua