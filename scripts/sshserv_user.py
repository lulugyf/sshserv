#coding=utf-8


import requests
import json
import sys
import urllib.parse as urlparse

userPath = "http://localhost:8080/api/v1/user"

def deluser(uname = "gg"):
    user = getusers(uname)
    if user is None:
        print("get user failed", uname)
        return
    userid = user['id']
    r = requests.delete(urlparse.urljoin(userPath, "user/" + str(userid)), auth=None,
                                    verify=True)
    print(r.status_code, r.text)

def getusers(username):
#     username = "laog1"
    r = requests.get(userPath, params={"limit":50, "offset":0, "order": 'ASC',
    											"username":username})
    print(r.status_code, r.text)
    if r.status_code == 200:
        j = json.loads(r.text)
        if len(j) > 0:
            return j[0]
    return None

def updateuser():
    uname = "caro"
    upass = "ll1"
    pubkey = ""
    #home = "/tmp"
    home = "/user/iasp"
    user = getusers(uname)
    if user is None:
        print(" get user failed or user not found!")
        return
    if pubkey != "":
        user["public_keys"] = pubkey
    if upass != "":
        user["password"] = upass
    if home != "":
        user['home_dir'] = home
    userid = user['id']
    r = requests.put(urlparse.urljoin(userPath, "user/" + str(userid)), json=user)
    print(" -- updateuser", r.status_code, r.text)

def adduser(uname="gg", upass="ll", home="/", pubkey="", perms=['*',]):
    userobj = {'id': 0, 'username': uname, 'uid': 0, 'gid': 0, 'max_sessions': 0,
               'quota_size': 0, 'quota_files': 0, 'upload_bandwidth': 0, 'download_bandwidth': 0,
               'home_dir': home,
               'permissions': perms,
               }
#               'permissions': ['list', 'download', 'upload', 'delete', 'rename', 'create_dirs']}
    if pubkey != "":
        userobj["public_keys"] = pubkey
    if upass != "":
        userobj["password"] = upass
    r = requests.post(userPath, json=userobj, auth=None, verify=True)
    print(r.status_code, r.text)

if __name__ == '__main__':
    if len(sys.argv) > 1:
        cmd = sys.argv[1]
        if cmd == 'add':
            adduser(uname=sys.argv[2])
        elif cmd == 'del':
            deluser(uname=sys.argv[2])
        elif cmd == 'list':
            getusers("")
    else:
        print("Usage: %s <add|del|list> [uname]" % sys.argv[0])
#     deluser()
#     getusers("laog1")
#    updateuser()
