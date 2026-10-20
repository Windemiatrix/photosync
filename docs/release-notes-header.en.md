<!-- release.yml substitutes {{TAG}} (the release tag) and {{ARCHIVE}} (the archive name: photosync_<version>_darwin_universal.tar.gz) -->

## How to verify and run

Requires macOS 13 or newer; the archive is universal, for Apple Silicon and Intel.

1. Download the archive and `checksums.txt` in a terminal (macOS then does not set the quarantine attribute):

   ```sh
   gh release download {{TAG}} -R Windemiatrix/photosync -p '{{ARCHIVE}}' -p checksums.txt
   ```

   Without `gh`:

   ```sh
   curl -LO https://github.com/Windemiatrix/photosync/releases/download/{{TAG}}/{{ARCHIVE}}
   curl -LO https://github.com/Windemiatrix/photosync/releases/download/{{TAG}}/checksums.txt
   ```

2. Verify the checksum. It protects only against corruption during download:

   ```sh
   grep '{{ARCHIVE}}' checksums.txt | shasum -a 256 -c -
   ```

3. Verify provenance: it confirms that the archive was built from this repository by the release workflow. Requires `gh`; without it authenticity cannot be verified.

   ```sh
   gh attestation verify '{{ARCHIVE}}' --repo Windemiatrix/photosync
   ```

4. Unpack and put the binary in `~/bin`:

   ```sh
   tar xzf '{{ARCHIVE}}'
   mkdir -p ~/bin && install -m 755 photosync ~/bin/photosync
   ```

   If `~/bin` is not in your `PATH`, add it or run `~/bin/photosync`.

5. Only if the file was downloaded by a browser and macOS blocks it, and only after the checks above, remove quarantine from this one file:

   ```sh
   xattr -d com.apple.quarantine ~/bin/photosync
   ```

6. Check the installation:

   ```sh
   photosync version
   photosync doctor
   ```

The binary has only an ad-hoc signature: it is not signed by Apple or notarized. After an update macOS may ask again for access to Photos and Keychain: this is expected, and `photosync doctor` shows the state.

The change list below is in English: it is built from PR titles.
