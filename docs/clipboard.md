# Clipboard Dependencies

AIBadger copies text to the system clipboard via a platform-native command.
Below are the required tools per platform.

## Linux

`xclip` with the `-selection clipboard` flag.

```bash
# Debian / Ubuntu / Linux Mint
sudo apt install xclip

# Fedora
sudo dnf install xclip

# Arch Linux
sudo pacman -S xclip

# openSUSE
sudo zypper install xclip
```
> [!IMPORTANT]
> `xclip` is not available on every Linux distribution by default. If it is
> missing, you will see a "clipboard copy failed" error. Install it using one
> of the commands above.

## macOS

`pbcopy` — built-in, no installation required.

## Windows

`clip` — built-in, no installation required.
