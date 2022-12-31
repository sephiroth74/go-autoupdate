# go-autoupdate

Auto update for golang console applications

Basic Example:

    myversion := "1.0.0" // for demostration
    app_filename := "absolute_executable_filename" 

    update := autoupdate.AutoUpdate{Options: autoupdate.Options{
        BaseUrl:  "http://example.com/pub",
        Version:  myversion,
        SelfName: app_filename,
    }}
    
    ch := update.InstallUpdate(*v, nil)
    result := <-ch
    if result != nil {
        panic(result)
        return
    }

    fmt.Printf("done. %s have been upgraded to version %s\n", filepath.Base(app_filename), v.Version)

# version json

Each os/arch update file must have a "version_{os}_{arch}.json" json file inside the *baseurl*.
For instance **version_darwin_arm64.json**, with this content template:

    {
        "version": "1.0.1",
        "datetime": "2022-12-31T15:33:48+0100",
        "checksum": "8b59266eeeb9fd3d1a5272154628f91701dd48a648ae7a725da61da31b45c87b",
        "size": 7776356,
        "path": "./relative/darwin_arm64/target_file.tgz"
    }

* **checksum** is the sha256 checksum of the *target_file.tgz*
* **size** is the filesize of the **target_file.tgz**
* **path** is the relative path (relative to the *baseUrl*) of the tgz update file
