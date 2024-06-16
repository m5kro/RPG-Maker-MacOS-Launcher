import sys
import os
import json
import subprocess
import requests
import zipfile
import shutil
import platform
import chardet
from evbunpack.__main__ import unpack_files
from PySide6.QtWidgets import (QApplication, QMainWindow, QPushButton, QFileDialog,
                               QVBoxLayout, QWidget, QMessageBox, QDialog, QComboBox,
                               QDialogButtonBox, QFormLayout)

class FolderPathApp(QMainWindow):
    def __init__(self):
        super().__init__()

        self.setWindowTitle("RPG Maker MV and MZ Launcher")
        self.resize(400, 200)

        self.central_widget = QWidget()
        self.setCentralWidget(self.central_widget)

        self.layout = QVBoxLayout(self.central_widget)

        self.select_button = QPushButton("Select Game Folder", self)
        self.layout.addWidget(self.select_button)

        self.install_button = QPushButton("Install/Update NWJS", self)
        self.layout.addWidget(self.install_button)
        self.install_button.clicked.connect(self.install_nwjs)

        if not self.check_nwjs_installed():
            self.select_button.setEnabled(False)
            self.select_button.setText("NWJS not installed")

        self.select_button.clicked.connect(self.select_folder)

    def check_nwjs_installed(self):
        applications_dir = os.path.expanduser("~/Applications")
        return "nwjs.app" in os.listdir(applications_dir)

    def select_folder(self):
        folder_path = QFileDialog.getExistingDirectory(self, "Select Game Folder")
        if folder_path:
            print("Selected folder path:", folder_path)
            if self.check_package_json(folder_path):
                print("Valid RPG Maker MV/MZ game folder.")
                self.launch_game(folder_path)
            else:
                QMessageBox.critical(self, "Error", "No package.json found in the selected folder.")

    def check_package_json(self, folder_path):
        file_path = os.path.join(folder_path, 'package.json')
        if os.path.exists(file_path):
            with open(file_path, 'rb') as file:
                raw_data = file.read()
                result = chardet.detect(raw_data)
                encoding = result['encoding']

            with open(file_path, 'r', encoding=encoding) as file:
                data = json.load(file)

            if 'name' in data and data['name'].strip() == '':
                data['name'] = 'tempname'

            with open(file_path, 'w', encoding=encoding) as file:
                json.dump(data, file, indent=4)

            print("JSON file updated successfully.")
            return True
        else:
            print("File does not exist.")
            return False

    def check_and_unpack_game_en(self, folder_path):
        game_exe = None

        for root, _, files in os.walk(folder_path):
            for file in files:
                if file.lower() == "game_en.exe":
                    game_exe = os.path.join(root, file)
                    break
            if game_exe:
                break

        if game_exe:
            extracted_dir = os.path.join(folder_path, "extracted")
            unpack_files(game_exe, extracted_dir, False, False)

            for item in os.listdir(extracted_dir):
                s = os.path.join(extracted_dir, item)
                d = os.path.join(folder_path, item)
                if os.path.isdir(s):
                    shutil.copytree(s, d, dirs_exist_ok=True)
                else:
                    shutil.copy2(s, d)

            os.remove(game_exe)
            shutil.rmtree(extracted_dir)
        else:
            print("game_en.exe not found in the directory")

    def launch_game(self, folder_path):
        nwjs_path = os.path.expanduser("~/Applications/nwjs.app/Contents/MacOS/nwjs")
        self.check_and_unpack_game_en(folder_path)
        subprocess.Popen([nwjs_path, folder_path])

    def install_nwjs(self):
        URL = "https://nwjs.io/versions"
        TARGET_DIR = os.path.expanduser("~/Applications/nwjs.app")

        def download_and_install(version, arch):
            url = f"https://dl.nwjs.io/{version}/nwjs-sdk-{version}-osx-{arch}.zip"
            tmp_file = "/tmp/nwjs.zip"

            print(f"Downloading and installing version {version} for {arch} architecture...")
            response = requests.get(url)
            if response.status_code != 200:
                print(f"Error: Failed to download version {version} for {arch}.")
                return

            with open(tmp_file, "wb") as f:
                f.write(response.content)

            subprocess.run(["unzip", "-q", tmp_file, "-d", "/tmp"])

            if os.path.exists(TARGET_DIR):
                shutil.rmtree(TARGET_DIR)
            
            shutil.move(f"/tmp/nwjs-sdk-{version}-osx-{arch}/nwjs.app", TARGET_DIR)
            print(f"Version {version} installed successfully at {TARGET_DIR}")

            os.remove(tmp_file)
            shutil.rmtree(f"/tmp/nwjs-sdk-{version}-osx-{arch}")

        arch = "x64" if platform.machine() == "x86_64" else "arm64"
        print("Querying available versions...")
        response = requests.get(URL)
        if response.status_code != 200:
            print("Error: Failed to query.")
            return

        data = response.json()
        versions = [v["version"] for v in data["versions"]]
        
        # Show version selection dialog
        version, ok = self.show_version_selection_dialog(versions)
        if ok and version:
            download_and_install(version, arch)
            if self.check_nwjs_installed():
                self.select_button.setEnabled(True)
                self.select_button.setText("Select Game Folder")
            else:
                self.select_button.setEnabled(False)
                self.select_button.setText("NWJS not installed")
            QMessageBox.information(self, "Install/Update NWJS", "NWJS installation/update completed successfully.")

    def show_version_selection_dialog(self, versions):
        dialog = QDialog(self)
        dialog.setWindowTitle("Select NWJS Version")
        layout = QFormLayout(dialog)
        
        combo_box = QComboBox(dialog)
        combo_box.addItems(versions)
        layout.addRow("Version:", combo_box)

        buttons = QDialogButtonBox(QDialogButtonBox.Ok | QDialogButtonBox.Cancel, dialog)
        buttons.accepted.connect(dialog.accept)
        buttons.rejected.connect(dialog.reject)
        layout.addRow(buttons)

        result = dialog.exec()
        return combo_box.currentText(), result == QDialog.Accepted

def check_appdir():
    app_dir = os.path.expanduser("~/Applications")
    if not os.path.exists(app_dir):
        os.makedirs(app_dir)

def main():
    check_appdir()
    app = QApplication(sys.argv)
    window = FolderPathApp()
    window.show()
    sys.exit(app.exec())

if __name__ == "__main__":
    main()
