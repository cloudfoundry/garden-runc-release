package users_test

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"code.cloudfoundry.org/guardian/rundmc/users"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

type lookupResult struct {
	u   *users.ExecUser
	err error
}

func lookupWithTimeout(root, name string) (*users.ExecUser, error) {
	GinkgoHelper()
	ch := make(chan lookupResult, 1)
	go func() {
		u, err := users.LookupUser(root, name)
		ch <- lookupResult{u: u, err: err}
	}()
	var res lookupResult
	Eventually(ch).WithTimeout(2*time.Second).Should(Receive(&res), "LookupUser did not return")
	return res.u, res.err
}

func mkfifoWithCleanup(path string) {
	Expect(syscall.Mkfifo(path, 0666)).To(Succeed())
	DeferCleanup(func() {
		fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			_ = syscall.Close(fd)
		}
	})
}

var _ = Describe("LookupUser", func() {
	Context("when we try to get the UID, GID and HOME of a username", func() {
		var (
			rootFsPath     string
			altDir         string
			uidOfTheBeast  = 666
			gidOfTheBeast  = 777
			homeOfTheBeast = "/home/fieryunderworld"
		)

		writeAltFile := func(name, content string) string {
			p := filepath.Join(altDir, name)
			Expect(os.MkdirAll(filepath.Dir(p), 0777)).To(Succeed())
			Expect(os.WriteFile(p, []byte(content), 0777)).To(Succeed())
			return p
		}

		writeRootfsFile := func(absPath, content string) string {
			p := filepath.Join(rootFsPath, absPath)
			Expect(os.MkdirAll(filepath.Dir(p), 0777)).To(Succeed())
			Expect(os.WriteFile(p, []byte(content), 0777)).To(Succeed())
			return p
		}

		createPasswdFile := func() error {
			return os.WriteFile(filepath.Join(rootFsPath, "etc", "passwd"), []byte(
				`_lda:*:211:211:Local Delivery Agent:/var/empty:/usr/bin/false
_cvmsroot:*:212:212:CVMS Root:/var/empty:/usr/bin/false
_usbmuxd:*:213:213:iPhone OS Device Helper:/var/db/lockdown:/usr/bin/false
devil:*:666:777:Beelzebub:/home/fieryunderworld:/usr/bin/false
_dovecot:*:214:6:Dovecot Administrator:/var/empty:/usr/bin/false
vcap:*:1000:1000:VCAP:/home/vcap/:/bin/sh`,
			), 0777)
		}

		createGroupFile := func() error {
			return os.WriteFile(filepath.Join(rootFsPath, "etc", "group"), []byte(
				`root:x:0:
daemon:x:1:
bin:x:2:
vcap:x:1000:
another:x:4:vcap`,
			), 0777)
		}

		BeforeEach(func() {
			var err error
			rootFsPath, err = os.MkdirTemp("", "passwdtestdir")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(os.RemoveAll, rootFsPath)
			altDir, err = os.MkdirTemp("", "passwdalt")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(os.RemoveAll, altDir)
			Expect(os.MkdirAll(filepath.Join(rootFsPath, "etc"), 0777)).To(Succeed())
			Expect(createGroupFile()).To(Succeed())
			Expect(createPasswdFile()).To(Succeed())
		})

		DescribeTable("user / group combinations", func(username string, expectedUid, expectedGid int, expectedHome string) {
			user, err := users.LookupUser(rootFsPath, username)
			Expect(err).ToNot(HaveOccurred())
			Expect(user.Uid).To(BeEquivalentTo(expectedUid))
			Expect(user.Gid).To(BeEquivalentTo(expectedGid))
			Expect(user.Home).To(Equal(expectedHome))
		},

			Entry("empty username", "", users.DefaultUID, users.DefaultGID, users.DefaultHome),
			Entry("username from /etc/passwd", "devil", uidOfTheBeast, gidOfTheBeast, homeOfTheBeast),
			Entry("username from /etc/passwd and groupname from /etc/group", "devil:daemon", uidOfTheBeast, 1, homeOfTheBeast),
			Entry("username from /etc/passwd and given groupid", "devil:123", uidOfTheBeast, 123, homeOfTheBeast),
			Entry("given userid and groupname from /etc/group", "666:vcap", uidOfTheBeast, 1000, homeOfTheBeast),
			Entry("given userid and groupid", "123:456", 123, 456, "/"),
			Entry("given userid that exists in /etc/passwd", "666", uidOfTheBeast, gidOfTheBeast, homeOfTheBeast),
		)

		Context("when /etc/passwd exists with no matching users", func() {
			It("returns an error", func() {
				_, err := users.LookupUser(rootFsPath, "unknownUser")
				Expect(err).To(MatchError(ContainSubstring("unable to find")))
			})
		})

		Context("when /etc/passwd exists but cannot be parsed", func() {
			BeforeEach(func() {
				senselessContents := []byte(
					`lorem ipsum dollar sit amet
					unix at the portal
					body type by letroset
					here at the epoch
					let us forget...`,
				)
				passwdPath := filepath.Join(rootFsPath, "etc", "passwd")
				Expect(os.WriteFile(passwdPath, senselessContents, 0777)).To(Succeed())
			})

			It("returns an error", func() {
				_, err := users.LookupUser(rootFsPath, "devil")
				Expect(err).To(MatchError(ContainSubstring("unable to find")))
			})
		})

		Context("when /etc/passwd does not exist", func() {
			BeforeEach(func() {
				Expect(os.RemoveAll(filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())
			})

			It("returns the default UID, GID and HOME when user 'root' is requested", func() {
				user, err := users.LookupUser(rootFsPath, "root")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(BeEquivalentTo(users.DefaultUID))
				Expect(user.Gid).To(BeEquivalentTo(users.DefaultGID))
				Expect(user.Home).To(Equal(users.DefaultHome))
				Expect(user.Sgids).To(BeEmpty())
			})

			It("tolerates a numeric uid", func() {
				user, err := users.LookupUser(rootFsPath, "123:456")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(Equal(123))
				Expect(user.Gid).To(Equal(456))
			})

			It("errors when a user other than root is requested", func() {
				_, err := users.LookupUser(rootFsPath, "nobody")
				Expect(err).To(MatchError(ContainSubstring("unable to find user nobody")))
			})
		})

		Context("when /etc/group does not exist", func() {
			BeforeEach(func() {
				Expect(os.RemoveAll(filepath.Join(rootFsPath, "etc", "group"))).To(Succeed())
			})

			It("tolerates a numeric gid", func() {
				_, err := users.LookupUser(rootFsPath, "123:456")
				Expect(err).NotTo(HaveOccurred())
			})

			It("errors when a group name is requested", func() {
				_, err := users.LookupUser(rootFsPath, "123:devil")
				Expect(err).To(MatchError(ContainSubstring("unable to find group devil")))
			})
		})

		Context("secondary groups", func() {
			It("has vcap in a secondary group", func() {
				user, err := users.LookupUser(rootFsPath, "vcap")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Gid).To(Equal(1000))
				Expect(user.Sgids).To(HaveLen(1))
				Expect(user.Sgids[0]).To(Equal(4))
			})
		})

		Context("when etc/passwd or etc/group are symlinks", func() {
			It("resolves an absolute etc/passwd symlink relative to the rootfs", func() {
				writeAltFile("passwd", "altuser:x:4242:4242::/altuser:/bin/sh\nroot:x:4242:4242::/altuser:/bin/sh\n")
				writeRootfsFile(filepath.Join(altDir, "passwd"), "root:x:0:0::/:/bin/sh\nlocaluser:x:5000:5000::/localuser:/bin/sh\n")
				Expect(os.Remove(filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())
				Expect(os.Symlink(filepath.Join(altDir, "passwd"), filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())

				rootUser, err := users.LookupUser(rootFsPath, "root")
				Expect(err).NotTo(HaveOccurred())
				Expect(rootUser.Uid).NotTo(Equal(4242))
				Expect(rootUser.Uid).To(Equal(0))

				_, err = users.LookupUser(rootFsPath, "altuser")
				Expect(err).To(MatchError(ContainSubstring("unable to find user altuser")))

				user, err := users.LookupUser(rootFsPath, "localuser")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(Equal(5000))
				Expect(user.Home).To(Equal("/localuser"))
			})

			It("resolves a relative etc/passwd symlink containing .. relative to the rootfs", func() {
				writeAltFile("passwd", "altuser:x:4242:4242::/altuser:/bin/sh\nroot:x:4242:4242::/altuser:/bin/sh\n")
				writeRootfsFile(filepath.Join(altDir, "passwd"), "root:x:0:0::/:/bin/sh\nlocaluser:x:5000:5000::/localuser:/bin/sh\n")
				Expect(os.Remove(filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())
				target := "../../../../../../../../../../../../../../../../" + strings.TrimPrefix(filepath.Join(altDir, "passwd"), "/")
				Expect(os.Symlink(target, filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())

				rootUser, err := users.LookupUser(rootFsPath, "root")
				Expect(err).NotTo(HaveOccurred())
				Expect(rootUser.Uid).NotTo(Equal(4242))
				Expect(rootUser.Uid).To(Equal(0))

				_, err = users.LookupUser(rootFsPath, "altuser")
				Expect(err).To(MatchError(ContainSubstring("unable to find user altuser")))

				user, err := users.LookupUser(rootFsPath, "localuser")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(Equal(5000))
				Expect(user.Home).To(Equal("/localuser"))
			})

			It("resolves a symlinked etc directory relative to the rootfs (absolute)", func() {
				writeAltFile("passwd", "altuser:x:4242:4242::/altuser:/bin/sh\nroot:x:4242:4242::/altuser:/bin/sh\n")
				writeAltFile("group", "altgrp:x:4343:localuser\n")
				writeRootfsFile(filepath.Join(altDir, "passwd"), "root:x:0:0::/:/bin/sh\nlocaluser:x:5000:5000::/localuser:/bin/sh\n")
				writeRootfsFile(filepath.Join(altDir, "group"), "localgrp:x:5353:localuser\n")
				Expect(os.RemoveAll(filepath.Join(rootFsPath, "etc"))).To(Succeed())
				Expect(os.Symlink(altDir, filepath.Join(rootFsPath, "etc"))).To(Succeed())

				rootUser, err := users.LookupUser(rootFsPath, "root")
				Expect(err).NotTo(HaveOccurred())
				Expect(rootUser.Uid).NotTo(Equal(4242))
				Expect(rootUser.Uid).To(Equal(0))

				_, err = users.LookupUser(rootFsPath, "altuser")
				Expect(err).To(MatchError(ContainSubstring("unable to find user altuser")))

				user, err := users.LookupUser(rootFsPath, "localuser")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(Equal(5000))
				Expect(user.Home).To(Equal("/localuser"))
				Expect(user.Sgids).To(ContainElement(5353))
				Expect(user.Sgids).NotTo(ContainElement(4343))

				_, err = users.LookupUser(rootFsPath, "localuser:altgrp")
				Expect(err).To(MatchError(ContainSubstring("unable to find group altgrp")))

				grpUser, err := users.LookupUser(rootFsPath, "localuser:localgrp")
				Expect(err).NotTo(HaveOccurred())
				Expect(grpUser.Gid).To(Equal(5353))
			})

			It("resolves a symlinked etc directory relative to the rootfs (relative ..)", func() {
				writeAltFile("passwd", "altuser:x:4242:4242::/altuser:/bin/sh\nroot:x:4242:4242::/altuser:/bin/sh\n")
				writeRootfsFile(filepath.Join(altDir, "passwd"), "root:x:0:0::/:/bin/sh\nlocaluser:x:5000:5000::/localuser:/bin/sh\n")
				Expect(os.RemoveAll(filepath.Join(rootFsPath, "etc"))).To(Succeed())
				target := "../../../../../../../../../../../../../../../../" + strings.TrimPrefix(altDir, "/")
				Expect(os.Symlink(target, filepath.Join(rootFsPath, "etc"))).To(Succeed())

				rootUser, err := users.LookupUser(rootFsPath, "root")
				Expect(err).NotTo(HaveOccurred())
				Expect(rootUser.Uid).NotTo(Equal(4242))
				Expect(rootUser.Uid).To(Equal(0))

				_, err = users.LookupUser(rootFsPath, "altuser")
				Expect(err).To(MatchError(ContainSubstring("unable to find user altuser")))

				user, err := users.LookupUser(rootFsPath, "localuser")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(Equal(5000))
				Expect(user.Home).To(Equal("/localuser"))
			})

			It("resolves an etc/group symlink relative to the rootfs", func() {
				writeAltFile("group", "altgrp:x:4343:\n")
				writeRootfsFile(filepath.Join(altDir, "group"), "localgrp:x:5353:\n")
				Expect(os.Remove(filepath.Join(rootFsPath, "etc", "group"))).To(Succeed())
				Expect(os.Symlink(filepath.Join(altDir, "group"), filepath.Join(rootFsPath, "etc", "group"))).To(Succeed())

				_, err := users.LookupUser(rootFsPath, "devil:altgrp")
				Expect(err).To(MatchError(ContainSubstring("unable to find group altgrp")))

				user, err := users.LookupUser(rootFsPath, "devil:localgrp")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Gid).To(Equal(5353))
			})

			It("treats an absolute symlink whose rootfs target is missing as a missing passwd", func() {
				writeAltFile("passwd", "altuser:x:4242:4242::/altuser:/bin/sh\nroot:x:4242:4242::/altuser:/bin/sh\n")
				Expect(os.Remove(filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())
				Expect(os.Symlink(filepath.Join(altDir, "passwd"), filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())

				user, err := users.LookupUser(rootFsPath, "root")
				Expect(err).NotTo(HaveOccurred())
				Expect(user).To(Equal(&users.ExecUser{Uid: 0, Gid: 0, Home: "/", Sgids: []int{}}))

				_, err = users.LookupUser(rootFsPath, "altuser")
				Expect(err).To(MatchError(ContainSubstring("unable to find user altuser")))
			})

			It("fails a named user and allows a numeric uid when etc/passwd is a symlink loop", func() {
				Expect(os.Remove(filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())
				Expect(os.Symlink("/etc/passwd", filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())

				_, err := users.LookupUser(rootFsPath, "devil")
				Expect(err).To(MatchError(ContainSubstring("unable to find user devil")))
				Expect(err.Error()).NotTo(ContainSubstring("too many levels"))

				_, err = users.LookupUser(rootFsPath, "root")
				Expect(err).To(MatchError(ContainSubstring("unable to find user root")))

				user, err := users.LookupUser(rootFsPath, "123:456")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(Equal(123))
				Expect(user.Gid).To(Equal(456))
			})

			It("fails a named user and allows a numeric uid when etc/passwd is a relative symlink loop", func() {
				Expect(os.Remove(filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())
				Expect(os.Symlink("passwd", filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())

				_, err := users.LookupUser(rootFsPath, "devil")
				Expect(err).To(MatchError(ContainSubstring("unable to find user devil")))
				Expect(err.Error()).NotTo(ContainSubstring("too many levels"))

				_, err = users.LookupUser(rootFsPath, "root")
				Expect(err).To(MatchError(ContainSubstring("unable to find user root")))
				Expect(err.Error()).NotTo(ContainSubstring("too many levels"))

				user, err := users.LookupUser(rootFsPath, "123:456")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(Equal(123))
				Expect(user.Gid).To(Equal(456))
			})

			It("treats a dangling symlink as missing passwd", func() {
				Expect(os.Remove(filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())
				Expect(os.Symlink("/does-not-exist-nonexistent", filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())

				user, err := users.LookupUser(rootFsPath, "root")
				Expect(err).NotTo(HaveOccurred())
				Expect(user).To(Equal(&users.ExecUser{Uid: 0, Gid: 0, Home: "/", Sgids: []int{}}))

				_, err = users.LookupUser(rootFsPath, "nobody")
				Expect(err).To(MatchError(ContainSubstring("unable to find user nobody")))
			})

			It("resolves a rootfs-local relative symlink", func() {
				Expect(os.MkdirAll(filepath.Join(rootFsPath, "usr", "share"), 0777)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(rootFsPath, "usr", "share", "passwd"), []byte("devil:*:666:777:Beelzebub:/home/fieryunderworld:/usr/bin/false\n"), 0777)).To(Succeed())
				Expect(os.Remove(filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())
				Expect(os.Symlink("../usr/share/passwd", filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())

				user, err := users.LookupUser(rootFsPath, "devil")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(Equal(666))
				Expect(user.Gid).To(Equal(777))
				Expect(user.Home).To(Equal("/home/fieryunderworld"))
			})

			It("resolves a rootfs-local absolute symlink", func() {
				Expect(os.WriteFile(filepath.Join(rootFsPath, "etc", "passwd.real"), []byte("devil:*:666:777:Beelzebub:/home/fieryunderworld:/usr/bin/false\n"), 0777)).To(Succeed())
				Expect(os.Remove(filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())
				Expect(os.Symlink("/etc/passwd.real", filepath.Join(rootFsPath, "etc", "passwd"))).To(Succeed())

				user, err := users.LookupUser(rootFsPath, "devil")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(Equal(666))
			})
		})

		Context("when etc/passwd is not a regular file", func() {
			DescribeTable("non-regular passwd file", func(setup func(string)) {
				p := filepath.Join(rootFsPath, "etc", "passwd")
				Expect(os.Remove(p)).To(Succeed())
				setup(p)

				_, err := lookupWithTimeout(rootFsPath, "devil")
				Expect(err).To(MatchError(ContainSubstring("unable to find user devil")))
				Expect(err.Error()).NotTo(ContainSubstring("not a regular file"))

				_, err = lookupWithTimeout(rootFsPath, "root")
				Expect(err).To(MatchError(ContainSubstring("unable to find user root")))

				user, err := lookupWithTimeout(rootFsPath, "123:456")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(Equal(123))
				Expect(user.Gid).To(Equal(456))
			},
				Entry("FIFO", func(p string) {
					mkfifoWithCleanup(p)
				}),
				Entry("char device", func(p string) {
					Expect(syscall.Mknod(p, syscall.S_IFCHR|0666, int(unix.Mkdev(1, 5)))).To(Succeed())
				}),
				Entry("unix socket", func(p string) {
					l, err := net.Listen("unix", p)
					Expect(err).NotTo(HaveOccurred())
					DeferCleanup(l.Close)
				}),
				Entry("directory", func(p string) {
					Expect(os.Mkdir(p, 0755)).To(Succeed())
				}),
				Entry("symlink to FIFO", func(p string) {
					target := filepath.Join(rootFsPath, "etc", "passwd.fifo")
					mkfifoWithCleanup(target)
					Expect(os.Symlink("passwd.fifo", p)).To(Succeed())
				}),
			)
		})

		Context("when etc/group is not a regular file", func() {
			DescribeTable("non-regular group file", func(setup func(string)) {
				p := filepath.Join(rootFsPath, "etc", "group")
				Expect(os.Remove(p)).To(Succeed())
				setup(p)

				user, err := lookupWithTimeout(rootFsPath, "123:456")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(Equal(123))
				Expect(user.Gid).To(Equal(456))

				_, err = lookupWithTimeout(rootFsPath, "123:daemon")
				Expect(err).To(MatchError(ContainSubstring("unable to find group daemon")))

				vcapUser, err := lookupWithTimeout(rootFsPath, "vcap")
				Expect(err).NotTo(HaveOccurred())
				Expect(vcapUser.Gid).To(Equal(1000))
				Expect(vcapUser.Sgids).To(BeEmpty())
			},
				Entry("FIFO", func(p string) {
					mkfifoWithCleanup(p)
				}),
				Entry("char device", func(p string) {
					Expect(syscall.Mknod(p, syscall.S_IFCHR|0666, int(unix.Mkdev(1, 5)))).To(Succeed())
				}),
				Entry("unix socket", func(p string) {
					l, err := net.Listen("unix", p)
					Expect(err).NotTo(HaveOccurred())
					DeferCleanup(l.Close)
				}),
				Entry("directory", func(p string) {
					Expect(os.Mkdir(p, 0755)).To(Succeed())
				}),
				Entry("symlink to FIFO", func(p string) {
					target := filepath.Join(rootFsPath, "etc", "group.fifo")
					mkfifoWithCleanup(target)
					Expect(os.Symlink("group.fifo", p)).To(Succeed())
				}),
			)
		})

		Context("when etc is a regular file", func() {
			BeforeEach(func() {
				Expect(os.RemoveAll(filepath.Join(rootFsPath, "etc"))).To(Succeed())
				Expect(os.WriteFile(filepath.Join(rootFsPath, "etc"), []byte("not a directory"), 0777)).To(Succeed())
			})

			It("errors when user 'root' is requested", func() {
				_, err := users.LookupUser(rootFsPath, "root")
				Expect(err).To(MatchError(ContainSubstring("unable to find user root")))
			})

			It("allows a numeric uid", func() {
				user, err := users.LookupUser(rootFsPath, "123:456")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(Equal(123))
				Expect(user.Gid).To(Equal(456))
			})
		})

		Context("when etc/passwd exceeds the size cap", func() {
			It("fails with an over-cap error for named and numeric users", func() {
				p := filepath.Join(rootFsPath, "etc", "passwd")
				f, err := os.OpenFile(p, os.O_WRONLY|os.O_TRUNC, 0777)
				Expect(err).NotTo(HaveOccurred())
				defer f.Close()

				firstLine := []byte("devil:*:666:777::/h:/bin/sh\n")
				_, err = f.Write(firstLine)
				Expect(err).NotTo(HaveOccurred())

				line := []byte("u:x:1:1::/:/bin/sh\n")
				written := len(firstLine)
				target := (10 << 20) + 100
				chunk := bytes.Repeat(line, 1024)
				for written < target {
					n, err := f.Write(chunk)
					Expect(err).NotTo(HaveOccurred())
					written += n
				}
				Expect(f.Sync()).To(Succeed())

				_, err = users.LookupUser(rootFsPath, "devil")
				Expect(err).To(MatchError(ContainSubstring("unable to find user devil")))
				Expect(err.Error()).To(ContainSubstring("file exceeds 10485760 bytes"))

				_, err = users.LookupUser(rootFsPath, "666")
				Expect(err).To(MatchError(ContainSubstring("file exceeds 10485760 bytes")))
			})

			It("fails when etc/passwd is exactly 10 MiB + 1 byte", func() {
				p := filepath.Join(rootFsPath, "etc", "passwd")
				f, err := os.OpenFile(p, os.O_WRONLY|os.O_TRUNC, 0777)
				Expect(err).NotTo(HaveOccurred())
				defer f.Close()

				firstLine := []byte("devil:*:666:777::/h:/bin/sh\n")
				_, err = f.Write(firstLine)
				Expect(err).NotTo(HaveOccurred())

				totalTarget := (10 << 20) + 1
				padLen := totalTarget - len(firstLine)
				line := []byte("# 123456789012345\n")
				written := 0
				chunk := bytes.Repeat(line, 512)
				for written+len(chunk) <= padLen-1 {
					n, err := f.Write(chunk)
					Expect(err).NotTo(HaveOccurred())
					written += n
				}
				rem := padLen - written
				remLine := make([]byte, rem)
				remLine[0] = '#'
				for i := 1; i < rem-1; i++ {
					remLine[i] = ' '
				}
				remLine[rem-1] = '\n'
				_, err = f.Write(remLine)
				Expect(err).NotTo(HaveOccurred())
				Expect(f.Sync()).To(Succeed())

				fi, err := f.Stat()
				Expect(err).NotTo(HaveOccurred())
				Expect(fi.Size()).To(Equal(int64((10 << 20) + 1)))

				_, err = users.LookupUser(rootFsPath, "devil")
				Expect(err).To(MatchError(ContainSubstring("unable to find user devil")))
				Expect(err.Error()).To(ContainSubstring("file exceeds 10485760 bytes"))

				_, err = users.LookupUser(rootFsPath, "666")
				Expect(err).To(MatchError(ContainSubstring("file exceeds 10485760 bytes")))
			})

			It("succeeds when etc/passwd is exactly 10 MiB", func() {
				p := filepath.Join(rootFsPath, "etc", "passwd")
				f, err := os.OpenFile(p, os.O_WRONLY|os.O_TRUNC, 0777)
				Expect(err).NotTo(HaveOccurred())
				defer f.Close()

				firstLine := []byte("devil:*:666:777::/h:/bin/sh\n")
				_, err = f.Write(firstLine)
				Expect(err).NotTo(HaveOccurred())

				totalTarget := 10 << 20
				padLen := totalTarget - len(firstLine)
				line := []byte("# 123456789012345\n")
				written := 0
				chunk := bytes.Repeat(line, 512)
				for written+len(chunk) <= padLen-1 {
					n, err := f.Write(chunk)
					Expect(err).NotTo(HaveOccurred())
					written += n
				}
				rem := padLen - written
				remLine := make([]byte, rem)
				remLine[0] = '#'
				for i := 1; i < rem-1; i++ {
					remLine[i] = ' '
				}
				remLine[rem-1] = '\n'
				_, err = f.Write(remLine)
				Expect(err).NotTo(HaveOccurred())
				Expect(f.Sync()).To(Succeed())

				fi, err := f.Stat()
				Expect(err).NotTo(HaveOccurred())
				Expect(fi.Size()).To(Equal(int64(10 << 20)))

				user, err := users.LookupUser(rootFsPath, "666")
				Expect(err).NotTo(HaveOccurred())
				Expect(user.Uid).To(Equal(666))
				Expect(user.Gid).To(Equal(777))
			})
		})

		Context("when etc/group exceeds the size cap", func() {
			It("fails with an over-cap error", func() {
				p := filepath.Join(rootFsPath, "etc", "group")
				f, err := os.OpenFile(p, os.O_WRONLY|os.O_TRUNC, 0777)
				Expect(err).NotTo(HaveOccurred())
				defer f.Close()

				line := []byte("g:x:100:\n")
				chunk := bytes.Repeat(line, 1024)
				written := 0
				target := (10 << 20) + 100
				for written < target {
					n, err := f.Write(chunk)
					Expect(err).NotTo(HaveOccurred())
					written += n
				}
				Expect(f.Sync()).To(Succeed())

				_, err = users.LookupUser(rootFsPath, "devil:daemon")
				Expect(err).To(MatchError(ContainSubstring("file exceeds 10485760 bytes")))
			})
		})
	})
})
