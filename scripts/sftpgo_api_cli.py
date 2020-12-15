#!/usr/bin/env python
import argparse
import json

import requests

try:
	import urllib.parse as urlparse
except ImportError:
	import urlparse

try:
	import pygments
	from pygments.lexers import JsonLexer
	from pygments.formatters import TerminalFormatter
except ImportError:
	pygments = None


class SFTPGoApiRequests:

	def __init__(self, debug, baseUrl, authType, authUser, authPassword, secure, no_color):
		self.userPath = urlparse.urljoin(baseUrl, '/api/v1/user')
		self.quotaScanPath = urlparse.urljoin(baseUrl, '/api/v1/quota_scan')
		self.activeConnectionsPath = urlparse.urljoin(baseUrl, '/api/v1/connection')
		self.versionPath = urlparse.urljoin(baseUrl, '/api/v1/version')
		self.debug = debug
		if authType == 'basic':
			self.auth = requests.auth.HTTPBasicAuth(authUser, authPassword)
		elif authType == 'digest':
			self.auth = requests.auth.HTTPDigestAuth(authUser, authPassword)
		else:
			self.auth = None
		self.verify = secure
		self.no_color = no_color

	def formatAsJSON(self, text):
		if not text:
			return ""
		json_string = json.dumps(json.loads(text), sort_keys=True, indent=2)
		if not self.no_color and pygments:
			return pygments.highlight(json_string, JsonLexer(), TerminalFormatter())
		return json_string

	def printResponse(self, r):
		if "content-type" in r.headers and "application/json" in r.headers["content-type"]:
			if self.debug:
				if pygments is None:
					print('')
					print('Response color highlight is not available: you need pygments 1.5 or above.')
				print('')
				print("Executed request: {} {} - request body: {}".format(
					r.request.method, r.url, self.formatAsJSON(r.request.body)))
				print('')
				print("Got response, status code: {} body:".format(r.status_code))
			print(self.formatAsJSON(r.text))
		else:
			print(r.text)

	def buildUserObject(self, user_id=0, username="", password="", public_keys="", home_dir="", uid=0,
					gid=0, max_sessions=0, quota_size=0, quota_files=0, permissions=[], upload_bandwidth=0,
					download_bandwidth=0):
		user = {"id":user_id, "username":username, "uid":uid, "gid":gid,
			"max_sessions":max_sessions, "quota_size":quota_size, "quota_files":quota_files,
			"upload_bandwidth":upload_bandwidth,"download_bandwidth":download_bandwidth}
		if password:
			user.update({"password":password})
		if public_keys:
			user.update({"public_keys":public_keys})
		if home_dir:
			user.update({"home_dir":home_dir})
		if permissions:
			user.update({"permissions":permissions})
		return user

	def getUsers(self, limit=100, offset=0, order="ASC", username=""):
		r = requests.get(self.userPath, params={"limit":limit, "offset":offset, "order":order,
											"username":username}, auth=self.auth, verify=self.verify)
		self.printResponse(r)

	def getUserByID(self, user_id):
		r = requests.get(urlparse.urljoin(self.userPath, "user/" + str(user_id)), auth=self.auth, verify=self.verify)
		self.printResponse(r)

	def addUser(self, username="", password="", public_keys="", home_dir="", uid=0, gid=0, max_sessions=0,
		quota_size=0, quota_files=0, permissions=[], upload_bandwidth=0, download_bandwidth=0):
		u = self.buildUserObject(0, username, password, public_keys, home_dir, uid, gid, max_sessions,
			quota_size, quota_files, permissions, upload_bandwidth, download_bandwidth)
		r = requests.post(self.userPath, json=u, auth=self.auth, verify=self.verify)
		self.printResponse(r)

	def updateUser(self, user_id, username="", password="", public_keys="", home_dir="", uid=0, gid=0,
				max_sessions=0, quota_size=0, quota_files=0, permissions=[], upload_bandwidth=0,
				download_bandwidth=0):
		u = self.buildUserObject(user_id, username, password, public_keys, home_dir, uid, gid, max_sessions,
			quota_size, quota_files, permissions, upload_bandwidth, download_bandwidth)
		r = requests.put(urlparse.urljoin(self.userPath, "user/" + str(user_id)), json=u, auth=self.auth, verify=self.verify)
		self.printResponse(r)

	def deleteUser(self, user_id):
		url = urlparse.urljoin(self.userPath, "user/" + str(user_id))
		print("----delete url", url)
		r = requests.delete(url, auth=self.auth, verify=self.verify)
		self.printResponse(r)

	def getConnections(self):
		r = requests.get(self.activeConnectionsPath, auth=self.auth, verify=self.verify)
		self.printResponse(r)

	def closeConnection(self, connectionID):
		r = requests.delete(urlparse.urljoin(self.activeConnectionsPath, "connection/" + str(connectionID)), auth=self.auth)
		self.printResponse(r)

	def getQuotaScans(self):
		r = requests.get(self.quotaScanPath, auth=self.auth, verify=self.verify)
		self.printResponse(r)

	def startQuotaScan(self, username):
		u = self.buildUserObject(0, username)
		r = requests.post(self.quotaScanPath, json=u, auth=self.auth, verify=self.verify)
		self.printResponse(r)

	def getVersion(self):
		r = requests.get(self.versionPath, auth=self.auth, verify=self.verify)
		self.printResponse(r)


def addCommonUserArguments(parser):
	parser.add_argument('username', type=str)
	parser.add_argument('-P', '--password', type=str, default="", help='Default: %(default)s')
	parser.add_argument('-K', '--rm ', type=str, nargs='+', default=[], help='Default: %(default)s')
	parser.add_argument('-H', '--home-dir', type=str, default="", help='Default: %(default)s')
	parser.add_argument('--uid', type=int, default=0, help='Default: %(default)s')
	parser.add_argument('--gid', type=int, default=0, help='Default: %(default)s')
	parser.add_argument('-C', '--max-sessions', type=int, default=0,
					help='Maximum concurrent sessions. 0 means unlimited. Default: %(default)s')
	parser.add_argument('-S', '--quota-size', type=int, default=0,
					help='Maximum size allowed as bytes. 0 means unlimited. Default: %(default)s')
	parser.add_argument('-F', '--quota-files', type=int, default=0, help="default: %(default)s")
	parser.add_argument('-G', '--permissions', type=str, nargs='+', default=[],
					choices=['*', 'list', 'download', 'upload', 'delete', 'rename', 'create_dirs',
							'create_symlinks'], help='Default: %(default)s')
	parser.add_argument('-U', '--upload-bandwidth', type=int, default=0,
					help='Maximum upload bandwidth as KB/s, 0 means unlimited. Default: %(default)s')
	parser.add_argument('-D', '--download-bandwidth', type=int, default=0,
					help='Maximum download bandwidth as KB/s, 0 means unlimited. Default: %(default)s')


if __name__ == '__main__':
	parser = argparse.ArgumentParser(formatter_class=argparse.ArgumentDefaultsHelpFormatter)
	parser.add_argument('-b', '--base-url', type=str, default='http://127.0.0.1:8080',
					help='Base URL for SFTPGo REST API. Default: %(default)s')
	parser.add_argument('-a', '--auth-type', type=str, default=None, choices=['basic', 'digest'],
					help='HTTP authentication type. Default: %(default)s')
	parser.add_argument("-u", "--auth-user", type=str, default="",
					help='User for HTTP authentication. Default: %(default)s')
	parser.add_argument('-p', '--auth-password', type=str, default='',
					help='Password for HTTP authentication. Default: %(default)s')
	parser.add_argument('-d', '--debug', dest='debug', action='store_true')
	parser.set_defaults(debug=False)
	parser.add_argument('-i', '--insecure', dest='secure', action='store_false',
					help='Set to false to ignore verifying the SSL certificate')
	parser.set_defaults(secure=True)
	parser.add_argument('-t', '--no-color', dest='no_color', action='store_true',
					help='Disable color highlight for JSON responses. You need python pygments module 1.5 or above to have highlighted output')
	parser.set_defaults(no_color=(pygments is None))

	subparsers = parser.add_subparsers(dest='command', help='sub-command --help')
	subparsers.required = True

	parserAddUser = subparsers.add_parser('add-user', help='Add a new SFTP user')
	addCommonUserArguments(parserAddUser)

	parserUpdateUser = subparsers.add_parser('update-user', help='Update an existing user')
	parserUpdateUser.add_argument('id', type=int, help='User\'s ID to update')
	addCommonUserArguments(parserUpdateUser)

	parserDeleteUser = subparsers.add_parser('delete-user', help='Delete an existing user')
	parserDeleteUser.add_argument('id', type=int, help='User\'s ID to delete')

	parserGetUsers = subparsers.add_parser('get-users', help='Returns an array with one or more SFTP users')
	parserGetUsers.add_argument('-L', '--limit', type=int, default=100, choices=range(1, 501),
							help='Maximum allowed value is 500. Default: %(default)s', metavar='[1...500]')
	parserGetUsers.add_argument('-O', '--offset', type=int, default=0, help='Default: %(default)s')
	parserGetUsers.add_argument('-U', '--username', type=str, default='', help='Default: %(default)s')
	parserGetUsers.add_argument('-S', '--order', type=str, choices=['ASC', 'DESC'], default='ASC',
							help='default: %(default)s')

	parserGetUserByID = subparsers.add_parser('get-user-by-id', help='Find user by ID')
	parserGetUserByID.add_argument('id', type=int)

	parserGetConnections = subparsers.add_parser('get-connections',
													help='Get the active users and info about their uploads/downloads')

	parserCloseConnection = subparsers.add_parser('close-connection', help='Terminate an active SFTP/SCP connection')
	parserCloseConnection.add_argument('connectionID', type=str)

	parserGetQuotaScans = subparsers.add_parser('get-quota-scans', help='Get the active quota scans')

	parserStartQuotaScans = subparsers.add_parser('start-quota-scan', help='Start a new quota scan')
	addCommonUserArguments(parserStartQuotaScans)

	parserGetVersion = subparsers.add_parser('get-version', help='Get version details')

	args = parser.parse_args()

	api = SFTPGoApiRequests(args.debug, args.base_url, args.auth_type, args.auth_user, args.auth_password, args.secure,
						 args.no_color)

	if args.command == 'add-user':
		api.addUser(args.username, args.password, args.public_keys, args.home_dir,
					args.uid, args.gid, args.max_sessions, args.quota_size, args.quota_files,
					args.permissions, args.upload_bandwidth, args.download_bandwidth)
	elif args.command == 'update-user':
		api.updateUser(args.id, args.username, args.password, args.public_keys, args.home_dir,
					args.uid, args.gid, args.max_sessions, args.quota_size, args.quota_files,
					args.permissions, args.upload_bandwidth, args.download_bandwidth)
	elif args.command == 'delete-user':
		api.deleteUser(args.id)
	elif args.command == 'get-users':
		api.getUsers(args.limit, args.offset, args.order, args.username)
	elif args.command == 'get-user-by-id':
		api.getUserByID(args.id)
	elif args.command == 'get-connections':
		api.getConnections()
	elif args.command == 'close-connection':
		api.closeConnection(args.connectionID)
	elif args.command == 'get-quota-scans':
		api.getQuotaScans()
	elif args.command == 'start-quota-scan':
		api.startQuotaScan(args.username)
	elif args.command == 'get-version':
		api.getVersion()

