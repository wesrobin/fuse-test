# Fuse Test - A FUSE-based File System with Caching

This project implements a simple read-only FUSE (Filesystem in Userspace) file system in Go. It uses a primary storage directory (simulating an NFS mount) and a secondary SSD directory for caching frequently accessed files.

Disclaimer, an LLM wrote this, so it is very wordy. Sorry.

## Features

* Read-only FUSE file system.
* Simulated NFS backend as the source of truth.
* SSD-based caching layer with different strategies:
    * Default: Caches all accessed files.
    * Size-Limited: Caches files up to a total size limit.
    * LRU (Least Recently Used): Evicts the least recently used files when capacity is reached.
* 'Dynamic' (on startup) loading of the file system structure from the NFS directory.
* Configurable via command-line flags.

## How to Run

1.  **Prerequisites**:
    * Go (built on v1.24.2).
    * Access to `apt-get`. The build script will attempt to install fuse3 using `sudo apt install -y fuse3` if `fusermount3` is not found.

3.  **Clone the repository or open a new Codespaces instance:**
    Apparently MacOSX doesn't play well with FUSE, so prefer Codespaces if you have a mac. If not, you can clone locally.
    ```bash
    git clone <your-repo-url>
    cd <your-repo-directory>
    ```

5.  **Make the build script executable (if it's not):**
    ```bash
    chmod +x build.sh
    ```

6.  **Run the build script:**
    ```bash
    ./build.sh
    ```
    * This script will:
        * Prompt you to enter the full path to a source directory whose contents will be copied into the `./nfs` directory. If left empty, it defaults to `testdata` which is included for convenience.
        * Check for and install `fuse3` if needed.
        * Build the Go application (output binary: `fuse-test`).
        * Create necessary directories: `nfs`, `ssd`, and `mnt/all-projects`.

7.  **Run the FUSE file system:**
    After the build script completes, you can run the application. The script summary will remind you:
    ```bash
    ./fuse-test
    ```
    This will mount the file system at `./mnt/all-projects`.

8.  **Access the mounted file system:**
    In another terminal, you can navigate to `./mnt/all-projects` and list or execute its contents:
    ```bash
    cd ./mnt/all-projects
    ls -R
    cat <some-file-from-your-nfs-source>
    python <some-file>.py # Runs the file.
    ```

9.  **Unmount the file system:**
    Press `Ctrl+C` to terminate `fuse-test`. You should see a log saying the file system was unmounted. If you don't or there are _any_ errors, you can use 
    ```bash
    fusermount3 -u /mnt/all-projects
    ```
    It's important this is done before running `./build.sh` again - since otherwise the build script will not be able to clean up the mountpoint.

## Usage Examples

The application accepts several command-line flags to configure its behavior. You can view flag options and defaults like:
```bash
./fuse-test -help
```
   
## Testing

For these tests, `./build.sh` with default source directory. Each test is a series of commands to run.

NOTE: If you see the following error:
```bash
ERROR: Failed to write to cache project-1/main.py: open /workspaces/fuse-test/ssd/<path$file_name>: permission denied. Proceeding without caching.
```
it's because you need to rebuild the directory structure (just run `build.sh` after unmounting the drive). Annoying I know.

1. **Files are mounted and readable, and runs python file**
```bash
<terminal 1>
./build.sh
./fuse-test

<terminal 2>
ls -R mnt/all-projects # Lists the entire directory with subfolders and files
cat mnt/all-projects/another-folder/text-file.txt # Prints out text. This should be slow (1s delay)
# Check terminal 1 to ensure that another-folder$text-file.txt has been cached. Look for NFS_READ followed by CACHE_LOADED.
cat mnt/all-projects/another-folder/text-file.txt # File should be cached, read should be instant
# Check terminal 1 to eensure that the cache was hit. Look for CACHE_HIT.
python mnt/all-projects/project-1/main.py # Should work provided python is installed, will be slow (1s delay).
# Check terminal 1 to ensure caching again. 
# NOTE: Running a file like with `python <file>` appears to read it twice (unlike with `cat`), so there is a miss and a hit logged here.

<terminal 1>
ctr+C
```

2. **Artificial size limit**
```bash
<terminal 1>
./build.sh
./fuse-test -cache=size -sizelim=64 # 64 bytes will be enough for some files, not for others. It will never be enough for 2.

<terminal 2>
cd mnt/all-projects
cat project-1/common-lib.py # 72 bytes, too big for our cache
# In terminal 1, check that the cache has rejected the file. Look for 'cache refused the file'
cat project-1/common-lib.py # Should still be slow
cat project-1/main.py # 54 bytes, we can cache
# In terminal 1, check that the file has been cached
cat project-1/main.py # Should be instant
cat another-folder/text-file.txt # 47 bytes so small enough for our cache, but it will be rejected because cache is full
# In terminal 1, check that the cache has rejected the file
cd ../.. # IMPORTANT. If you don't cd out before stopping the go process, it will not unmount the drive. If so, run `fusermount3 -u mnt/all-projects` and you'll need to rebuild the binary with ./build.sh (sorry)

<terminal 1>
ctr+C
```

3. **LRU Cache**
```bash
<terminal 1>
./build.sh
./fuse-test -cache=lru -lrucap=2 -lrudebug # 2 files in cache at any one time, evicted by LRU

<terminal 2>
cd mnt/all-projects
cat project-1/common-lib.py # Cached
# In terminal 1, check that the file has been cached
cat project-1/common-lib.py # Should be instant
cat project-1/main.py # Cached
# In terminal 1, check that the file has been cached. Look for LRU_DEBUG log to verify the order is: [common-lib.py main.py]
cat another-folder/text-file.txt # Cached, and cache should evict common-lib.py
# In terminal 1, check that the file has been cached, and order is: [main.py, text-file.txt]
cd ../.. # IMPORTANT. If you don't cd out before stopping the go process, it will not unmount the drive. If so, run `fusermount3 -u mnt/all-projects` and you'll need to rebuild the binary with ./build.sh (sorry)

<terminal 1>
ctr+C
```

4. **Permissions**
```bash
<terminal 1>
./build.sh
./fuse-test

<terminal 2>
sudo useradd newuser
sudo passwd newuser # Leave empty, I used “123”
su - newuser # Enter password after prompt
# Now you’re some random user. Confirm you can’t see the stuffs
ls /workspaces/cerebrium-test/mnt/all-projects
# Should fail with “ls: cannot open directory 'mnt/all-projects/': Permission denied”
exit

<terminal 1>
ctr+C
```

5. **Read-only**
```bash
<terminal 1>
./build.sh
./fuse-test

<terminal 2>
rm -rf mnt/all-projects/project-1
# Should fail with "rm: cannot remove 'mnt/all-projects/project-1/common-lib.py': Read-only file system" for both files
touch mnt/all-projects/another-folder/test.txt
# Should fail with "touch: cannot touch 'mnt/all-projects/another-folder/test.txt': Read-only file system"

<terminal 1>
ctr+C
```

## File System Design

The file system is designed as a read-only layer that sits on top of an existing directory structure (referred to as "NFS") and uses another directory ("SSD") for caching.

1.  **NFS Directory (`./nfs`)**
    * This directory simulates a network file share or a primary, slower storage.
    * The file system structure (directories and files) is initially built by walking this directory when the `fuse-test` application starts (`loadFSTree` in `fs.go`).
    * All file metadata (like size, permissions, and modification times via `stat()`) is derived from the files in this NFS directory.
    * When a file is requested and not found in the cache, it is read directly from here, with a simulated delay (`nfsFileReadDelay`) to mimic network latency.

2.  **SSD Cache Directory (`./ssd`)**
    * This directory acts as a faster, local cache.
    * When a file is read from the NFS directory, its contents are subsequently stored in the SSD cache.
    * Subsequent reads for the same file will first attempt to fetch from the SSD cache. If found (cache hit), this avoids the slower NFS read.
    * Cache implementations (`cache.go`):
        * `defaultCache`: A simple pass-through cache. It writes files to the SSD directory but doesn't have eviction logic beyond overwriting.
        * `sizeLimitedCache`: This cache refuses to cache new files if the configured size limit is breached upon a new `Put`.
        * `lruCache`: Implements a Least Recently Used eviction policy. It maintains a queue of file paths. When a file is accessed (`Get`) or added (`Put`), it's moved to the back of the queue (most recently used). If the queue exceeds its `capacity` (number of files), the file path at the front (least recently used) is evicted, and the corresponding file is removed from the SSD directory. A map is also maintained as a means to quickly check if a given file is present, since iterating the queue is slow.
    * **Path Flattening**: To store files from a nested directory structure into the single SSD cache directory, paths are "flattened" by replacing `/` characters with `$` (e.g., `project-1/main.py` becomes `project-1$main.py` in the cache). Each file is then stored in the base `ssd` folder.

3.  **FUSE Implementation (`fs.go`, `node.go`)**
    * The system uses the `bazil.org/fuse` library.
    * `fuseFS` is the main struct representing the file system instance. It handles mounting, serving requests, and unmounting.
    * `fuseFSNode` represents an individual file or directory within the FUSE system. Each node has an inode number, mode, and methods to handle FUSE operations like `Attr` (get attributes), `Lookup` (find a file in a directory), `ReadDirAll` (list directory contents), and `Read` (read file contents).
    * Inodes are generated by a simple incrementing counter (`GenerateInode` in `fs.go`).
    * The entire file system is mounted as read-only (`fuse.ReadOnly()`).

## Known Issues

* FUSE calls read twice when executing a file (eg python <filepath>)
    * I _think_ this is expected, cat only does it once so it’s possible that executing reads the file once, and again to run? I’m not completely sure so including it here.
* Codespaces might randomly start bugging out complaining of some Go version.
    * If this happens, you need to tear down the code spaces instance. It’s fatal.
* Panics cause a lot of havoc. There’s no guarantee the mounted directory gets unmounted. In this case manually unmount and rebuild to wipe the directories. I should hope there aren't ways to make this thing panic but you never know.
* “Device or resource busy”: sometimes fuse can’t unmount the dir when the go binary is terminated, even when nothing is obviously using mnt/all-projects. I’m not sure why this happens, possible the OS (or VSCode) is doing something precisely during the unmount time.
    * `fusermount3 -u met/all-projects` + `./build.sh` mostly works to reset the environment, but in my experience sometimes it is unrecoverable, time to tear down Codespaces (I’m sure there’s a better way to do this)
* LRU cache is written with a backing array, not a doubly linked list. Means it’s a bit slower, but implementation in go was easier.

## Further Improvements

* Updates made to the NFS directory after mounting are currently reflected in the FUSE mount.
* I did not manage to get around to caching based on a hash of file contents.
* LRU cache implementation is a bit naive. It can be improved a bunch.
* In fact, in general I think the way the file system interacts with the cache is a little undercooked. Lots of improvements that can be made here.
* File contents are read from NFS into memory, then written into the corresponding SSD file. This will be really slow and potentially not work with giant files.
* I think seprarting File and Dir types that implement the fs.Node interface would be preferable. There's a bunch of `if isDir` checks in the code, this could be easily avoided.
* Error handling could be better. Most of the time, errors are just bubbled up to the FUSE server. Greater care should be taken to map errors to linux error codes. In most obvious cases, I have done this, but it still feels a little half-baked.
* Logging is verbose and annoying. I've tried to turn most of the logging off by putting it behind flags. A proper logger like [zap](https://github.com/uber-go/zap) would be far better.

Also, shoutout to [https://github.com/goamaral/fuse-filesystem](https://github.com/goamaral/fuse-filesystem), whoever you are!
