use std::error::Error;

fn main() -> Result<(), Box<dyn Error>> {
    let mut prost = prost_build::Config::new();
    prost.protoc_executable(protoc_bin_vendored::protoc_bin_path()?);
    tonic_prost_build::configure().compile_with_config(
        prost,
        &["../../contracts/financial-engine/financial_engine.proto"],
        &["../../contracts/financial-engine"],
    )?;
    println!("cargo:rerun-if-changed=../../contracts/financial-engine/financial_engine.proto");
    Ok(())
}
